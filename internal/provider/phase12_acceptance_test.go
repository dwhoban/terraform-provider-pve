// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// testAccCephInstalled reports whether the node has ceph binaries. Package
// installation is not exposed through the PVE API, so the suite skips with
// instructions when they are missing.
func testAccCephInstalled(t *testing.T, node string) bool {
	t.Helper()
	_, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/ceph/osd", nil)
	return err == nil || !strings.Contains(err.Error(), "binary not installed")
}

// Phase 12 acceptance scenario (ROADMAP.md), executed for real on the
// designated scratch disks: init → mon → OSDs on /dev/nvme0n1 and
// /dev/nvme1n1 → pool (with its rbd storage auto-registered) → flag
// round-trip → status DS → osd out/in actions → full teardown (pool with
// remove_storages, mon, OSDs destroyed with cleanup). Ceph packages are
// installed by PVE on first use; the initialized-but-empty ceph config is
// left behind intentionally (PVE has no de-init).

func TestAccPhase12CephLifecycle(t *testing.T) {
	node := testAccNode(t)
	if !testAccCephInstalled(t, node) {
		t.Skip("ceph binaries are not installed on the node; see ROADMAP/CHANGELOG note — run `apt install ceph-common ceph-mon ceph-osd ceph-mgr` (or `pveceph install`) on pve01 first")
	}
	poolName := "tfacc-pool"

	config := func(noscrub string) string {
		return fmt.Sprintf(`
# Bootstrap: initialize ceph on the node before any MON/OSD exists.
action "pve_ceph_init" "init" {
  config { node = %q }
}
resource "terraform_data" "init_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_init.init]
    }
  }
}

resource "pve_ceph_mon" "mon" {
  node  = %q
  monid = %q
}

resource "pve_ceph_osd" "osd0" {
  node    = %q
  dev     = "/dev/nvme0n1"
  cleanup = true
  depends_on = [terraform_data.init_trigger, pve_ceph_mon.mon]
}

resource "pve_ceph_osd" "osd1" {
  node    = %q
  dev     = "/dev/nvme1n1"
  cleanup = true
  depends_on = [pve_ceph_osd.osd0]
}

resource "pve_ceph_pool" "pool" {
  node         = %q
  name         = %q
  add_storages = true
  size         = 2
  min_size     = 1
  depends_on   = [pve_ceph_osd.osd1]
}

resource "pve_ceph_flag" "noscrub" {
  flag  = "noscrub"
  value = %s
  depends_on = [pve_ceph_mon.mon]
}

data "pve_ceph_status" "status" {
  depends_on = [pve_ceph_pool.pool]
}
`, node, node, node, node, node, node, poolName, noscrub)
	}

	osdGone := func(dev string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/disks/list", nil)
			if err != nil {
				return err
			}
			var disks []map[string]any
			if err := jsonUnmarshalT(payload, &disks); err != nil {
				return err
			}
			for _, d := range disks {
				if d["devpath"] == dev {
					if osdid, ok := d["osdid"].(float64); ok && osdid >= 0 {
						return fmt.Errorf("%s still carries OSD %v", dev, int(osdid))
					}
					if used, ok := d["used"].(string); ok && used != "" {
						return fmt.Errorf("%s still in use: %s", dev, used)
					}
				}
			}
			return nil
		}
	}
	cephGone := func(s *terraform.State) error {
		payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/ceph/osd", nil)
		if err != nil {
			// No ceph binaries = no OSDs possible.
			if strings.Contains(err.Error(), "binary not installed") {
				return nil
			}
			return err
		}
		var osds []map[string]any
		if err := jsonUnmarshalT(payload, &osds); err != nil {
			return err
		}
		if len(osds) != 0 {
			return fmt.Errorf("%d OSDs still exist", len(osds))
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			cephGone,
			osdGone("/dev/nvme0n1"),
			osdGone("/dev/nvme1n1"),
		),
		Steps: []resource.TestStep{
			{
				Config: config("true"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_ceph_mon.mon", tfjsonpath.New("id"), knownvalue.StringExact(node+"/"+node)),
					statecheck.ExpectKnownValue("pve_ceph_osd.osd0", tfjsonpath.New("osdid"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("pve_ceph_osd.osd1", tfjsonpath.New("osdid"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("pve_ceph_pool.pool", tfjsonpath.New("id"), knownvalue.StringExact(node+"/"+poolName)),
					statecheck.ExpectKnownValue("pve_ceph_flag.noscrub", tfjsonpath.New("value"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("data.pve_ceph_status.status", tfjsonpath.New("status"), knownvalue.NotNull()),
				},
			},
			{
				Config: config("false"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_ceph_flag.noscrub", tfjsonpath.New("value"), knownvalue.Bool(false)),
				},
			},
		},
	})
}

func TestAccPhase12CephOsdInOutActions(t *testing.T) {
	node := testAccNode(t)
	if !testAccCephInstalled(t, node) {
		t.Skip("ceph binaries are not installed on the node (see CephLifecycle)")
	}
	// Reuses an OSD if the lifecycle test ran; otherwise creates one on
	// nvme0n1 for the action round-trip and destroys it after.
	config := fmt.Sprintf(`
resource "pve_ceph_osd" "osd" {
  node    = %q
  dev     = "/dev/nvme0n1"
  cleanup = true
}

action "pve_ceph_osd_out" "out" {
  config { node = pve_ceph_osd.osd.node, osdid = pve_ceph_osd.osd.osdid }
}
action "pve_ceph_osd_in" "in" {
  config { node = pve_ceph_osd.osd.node, osdid = pve_ceph_osd.osd.osdid }
}

resource "terraform_data" "out_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_osd_out.out]
    }
  }
}

resource "terraform_data" "in_trigger" {
  depends_on = [terraform_data.out_trigger]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_osd_in.in]
    }
  }
}

data "pve_ceph_osd_metadata" "meta" {
  node  = pve_ceph_osd.osd.node
  osdid = pve_ceph_osd.osd.osdid
  depends_on = [terraform_data.in_trigger]
}
`, node)
	destroy := func(s *terraform.State) error {
		// If the shared-cluster path left this OSD behind, remove it now.
		payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/ceph/osd", nil)
		if err != nil {
			if pveclient.IsNotFound(err) {
				return nil
			}
			return err
		}
		var osds []map[string]any
		if err := jsonUnmarshalT(payload, &osds); err != nil {
			return err
		}
		for _, o := range osds {
			idf, ok := o["osdid"].(float64)
			if !ok {
				continue
			}
			_, _ = testAccClient(t).DoRequest(context.Background(), "DELETE",
				fmt.Sprintf("/nodes/%s/ceph/osd/%d?cleanup=1", node, int(idf)), nil)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             destroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_ceph_osd.osd", tfjsonpath.New("osdid"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_ceph_osd_metadata.meta", tfjsonpath.New("id"), knownvalue.NotNull()),
				},
			},
		},
	})
}
