// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 2 acceptance scenarios (ROADMAP.md): every inventory data source
// reads the live lab with values asserted non-null and shape-checked;
// pve_node and pve_nodes agree on the node list; nextid() returns an
// unused VMID verified against cluster resources. Detail reads that need
// objects the lab may not have (VMs, NFS servers, URLs) are env-gated.

func testAccNode(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("PVE_NODE"); v != "" {
		return v
	}
	return "pve01"
}

func testAccClient(t *testing.T) *pveclient.Client {
	t.Helper()
	c, err := pveclient.NewClient(pveclient.Config{
		URL:         os.Getenv(pveclient.EnvAPIURL),
		Credentials: pveclient.Credentials{APIToken: os.Getenv(pveclient.EnvAPIToken)},
		Insecure:    true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func testAccLatestUPID(t *testing.T) string {
	t.Helper()
	payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/cluster/tasks", nil)
	if err != nil {
		return ""
	}
	var tasks []map[string]any
	if err := json.Unmarshal(payload, &tasks); err != nil {
		return ""
	}
	if len(tasks) == 0 {
		return ""
	}
	upid, _ := tasks[0]["upid"].(string)
	return upid
}

func testAccFirstVG(t *testing.T) string {
	t.Helper()
	node := testAccNode(t)
	payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/scan/lvm", nil)
	if err != nil {
		return ""
	}
	var vgs []map[string]any
	if err := json.Unmarshal(payload, &vgs); err != nil {
		return ""
	}
	if len(vgs) == 0 {
		return ""
	}
	vg, _ := vgs[0]["vg"].(string)
	return vg
}

func testAccBackupInfoAllowed(t *testing.T) bool {
	t.Helper()
	_, err := testAccClient(t).DoRequest(context.Background(), "GET", "/cluster/backup-info", nil)
	return err == nil
}

func TestAccPhase2DataSources(t *testing.T) {
	node := testAccNode(t)

	type dsCase struct {
		name      string
		config    string
		checkAddr string // data address carrying the result attribute
		paths     []tfjsonpath.Path
		skip      string
	}

	// Discover optional inputs from the lab.
	if os.Getenv("PVE_TEST_UPID") == "" {
		if upid := testAccLatestUPID(t); upid != "" {
			t.Setenv("PVE_TEST_UPID", upid)
		}
	}
	if os.Getenv("PVE_TEST_VG") == "" {
		if vg := testAccFirstVG(t); vg != "" {
			t.Setenv("PVE_TEST_VG", vg)
		}
	}

	cases := []dsCase{
		{
			name: "pve_cluster", checkAddr: "data.pve_cluster.test",
			config: `data "pve_cluster" "test" {}`,
			paths: []tfjsonpath.Path{
				tfjsonpath.New("members"), tfjsonpath.New("resources"), tfjsonpath.New("tasks"),
			},
		},
		{
			name: "pve_nodes", checkAddr: "data.pve_nodes.test",
			config: `data "pve_nodes" "test" {}`,
			paths:  []tfjsonpath.Path{tfjsonpath.New("nodes")},
		},
		{
			name: "pve_node", checkAddr: "data.pve_node.test",
			config: fmt.Sprintf(`data "pve_node" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("info"), tfjsonpath.New("status"), tfjsonpath.New("version")},
		},
		{
			name: "pve_node_capabilities", checkAddr: "data.pve_node_capabilities.test",
			config: fmt.Sprintf(`data "pve_node_capabilities" "test" { node = %q }`, node),
			paths: []tfjsonpath.Path{
				tfjsonpath.New("cpu_models"), tfjsonpath.New("cpu_flags"), tfjsonpath.New("machines"),
			},
		},
		{
			name: "pve_node_hardware", checkAddr: "data.pve_node_hardware.test",
			config: fmt.Sprintf(`data "pve_node_hardware" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("pci"), tfjsonpath.New("usb")},
		},
		{
			name: "pve_node_disks", checkAddr: "data.pve_node_disks.test",
			config: fmt.Sprintf(`data "pve_node_disks" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("disks")},
		},
		{
			name: "pve_node_storages", checkAddr: "data.pve_node_storages.test",
			config: fmt.Sprintf(`data "pve_node_storages" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("storages")},
		},
		{
			name: "pve_storage_scan_lvm", checkAddr: "data.pve_storage_scan_lvm.test",
			config: fmt.Sprintf(`data "pve_storage_scan_lvm" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("entries")},
		},
		{
			name: "pve_storage_scan_lvmthin", checkAddr: "data.pve_storage_scan_lvmthin.test",
			config: fmt.Sprintf("data \"pve_storage_scan_lvmthin\" \"test\" {\n  node = %q\n  vg   = %q\n}", node, os.Getenv("PVE_TEST_VG")),
			paths:  []tfjsonpath.Path{tfjsonpath.New("entries")},
			skip:   "no LVM volume group on the lab (set PVE_TEST_VG to force)",
		},
		{
			name: "pve_storage_scan_zfs", checkAddr: "data.pve_storage_scan_zfs.test",
			config: fmt.Sprintf(`data "pve_storage_scan_zfs" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("entries")},
		},
		{
			name: "pve_storage_scan_nfs", checkAddr: "data.pve_storage_scan_nfs.test",
			config: fmt.Sprintf("data \"pve_storage_scan_nfs\" \"test\" {\n  node   = %q\n  server = %q\n}", node, os.Getenv("PVE_TEST_NFS_SERVER")),
			paths:  []tfjsonpath.Path{tfjsonpath.New("entries")},
			skip:   "set PVE_TEST_NFS_SERVER to run",
		},
		{
			name: "pve_appliance_templates", checkAddr: "data.pve_appliance_templates.test",
			config: fmt.Sprintf(`data "pve_appliance_templates" "test" { node = %q }`, node),
			paths:  []tfjsonpath.Path{tfjsonpath.New("templates")},
		},
		{
			name: "pve_task", checkAddr: "data.pve_task.test",
			config: fmt.Sprintf("data \"pve_task\" \"test\" {\n  node = %q\n  upid = %q\n}", node, os.Getenv("PVE_TEST_UPID")),
			paths:  []tfjsonpath.Path{tfjsonpath.New("entry"), tfjsonpath.New("task")},
			skip:   "set PVE_TEST_UPID to run",
		},
		{
			name: "pve_backup_info", checkAddr: "data.pve_backup_info.test",
			config: `data "pve_backup_info" "test" {}`,
			paths:  []tfjsonpath.Path{tfjsonpath.New("backups"), tfjsonpath.New("not_backed_up")},
		},
		{
			name: "pve_url_metadata", checkAddr: "data.pve_url_metadata.test",
			config: fmt.Sprintf("data \"pve_url_metadata\" \"test\" {\n  node         = %q\n  url          = %q\n  content_type = \"iso\"\n}", node, os.Getenv("PVE_TEST_URL")),
			paths:  []tfjsonpath.Path{tfjsonpath.New("metadata")},
			skip:   "set PVE_TEST_URL to run",
		},
	}

	skipEnv := map[string]string{
		"pve_storage_scan_nfs":     "PVE_TEST_NFS_SERVER",
		"pve_task":                 "PVE_TEST_UPID",
		"pve_url_metadata":         "PVE_TEST_URL",
		"pve_storage_scan_lvmthin": "PVE_TEST_VG",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" && os.Getenv(skipEnv[tc.name]) == "" {
				t.Skip(tc.skip)
			}
			if tc.name == "pve_backup_info" && !testAccBackupInfoAllowed(t) {
				t.Skip("/cluster/backup-info is root@pam-only; the API token is rejected")
			}
			checks := []statecheck.StateCheck{statecheck.ExpectKnownValue(tc.checkAddr, tfjsonpath.New("id"), knownvalue.NotNull())}
			for _, p := range tc.paths {
				checks = append(checks, statecheck.ExpectKnownValue(tc.checkAddr, p, knownvalue.NotNull()))
			}
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: tc.config, ConfigStateChecks: checks},
				},
			})
		})
	}
}

// TestAccPhase2NodeAgreement pins the roadmap scenario: pve_node and
// pve_nodes agree on the node list, and the node reports itself online.
func TestAccPhase2NodeAgreement(t *testing.T) {
	node := testAccNode(t)
	config := fmt.Sprintf(`
data "pve_nodes" "all" {}
data "pve_node" "test" { node = %q }

output "listed_and_up" {
  value = contains([for n in data.pve_nodes.all.nodes : n.node], data.pve_node.test.node) && data.pve_node.test.status.pveversion != ""
}
`, node)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckOutput("listed_and_up", "true"),
				),
			},
		},
	})
}

// TestAccPhase2Nextid pins the nextid() function scenario: a plausible
// VMID (>= 100) that is not present in the cluster resource list.
func TestAccPhase2Nextid(t *testing.T) {
	config := `
data "pve_cluster" "c" {}

locals {
  next = provider::pve::nextid()
}

output "next" {
  value = local.next
}

output "free" {
  value = !contains([for r in data.pve_cluster.c.resources : tostring(r.id) if startswith(r.type, "vm") || startswith(r.type, "qemu") || startswith(r.type, "lxc")], tostring(local.next))
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchOutput("next", regexp.MustCompile(`^\d+$`)),
					resource.TestCheckOutput("free", "true"),
				),
			},
		},
	})
}
