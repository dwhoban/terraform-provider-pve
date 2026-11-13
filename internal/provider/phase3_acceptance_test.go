// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 3 acceptance scenarios (ROADMAP.md): VM lifecycle (create, hot
// config updates, import), auto-allocated VMID, create-from-ISO with the
// start action, snapshot create + rollback, and full clone-at-create.
// Guests are diskless scratch VMs in the 90000-98999 range (except the
// auto-ID case, which lets PVE allocate).

func testAccVMID() int {
	return 90000 + acctest.RandIntRange(0, 8999)
}

func testAccVMGone(t *testing.T, node string, vmid int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		path := fmt.Sprintf("/nodes/%s/qemu/%d/status/current", node, vmid)
		_, err := testAccClient(t).DoRequest(context.Background(), "GET", path, nil)
		if pveclient.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("VM %d still exists", vmid)
	}
}

func TestAccPhase3VmLifecycle(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-vm-%d", vmid)
	config := func(memory, cores int) string {
		return fmt.Sprintf(`
resource "pve_vm" "test" {
  node   = %q
  vmid   = %d
  name   = %q
  memory = %d
  cores  = %d
}
`, node, vmid, name, memory, cores)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccVMGone(t, node, vmid),
		Steps: []resource.TestStep{
			{
				Config: config(512, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("id"), knownvalue.StringExact(fmt.Sprintf("%s/%d", node, vmid))),
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("vmid"), knownvalue.Int64Exact(int64(vmid))),
					// memory accepts unit suffixes ("512MiB") and is echoed
					// as a string; cores is numeric.
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("memory"), knownvalue.StringExact("512")),
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("cores"), knownvalue.Int64Exact(1)),
				},
			},
			{
				Config: config(1024, 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("memory"), knownvalue.StringExact("1024")),
					statecheck.ExpectKnownValue("pve_vm.test", tfjsonpath.New("cores"), knownvalue.Int64Exact(2)),
				},
			},
			{
				ResourceName:            "pve_vm.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"vmgenid", "smbios1", "boot"}, // server-generated when unset
			},
		},
	})
}

func TestAccPhase3VmFromISOWithStart(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-iso-%d", vmid)
	vmRunning := func(running bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			var status struct {
				Status string `json:"status"`
			}
			payload, err := testAccClient(t).DoRequest(context.Background(), "GET", fmt.Sprintf("/nodes/%s/qemu/%d/status/current", node, vmid), nil)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(payload, &status); err != nil {
				return err
			}
			want := "stopped"
			if running {
				want = "running"
			}
			if status.Status != want {
				return fmt.Errorf("vm status = %q, want %q", status.Status, want)
			}
			return nil
		}
	}
	config := func(withStop bool) string {
		stop := ""
		if withStop {
			stop = `
action "pve_vm_stop" "s" {
  config {
    node = pve_vm.test.node
    vmid = pve_vm.test.vmid
  }
}

resource "terraform_data" "stop_trigger" {
  depends_on = [terraform_data.start_trigger]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_stop.s]
    }
  }
}
`
		}
		return fmt.Sprintf(`
resource "pve_vm" "test" {
  node   = %q
  vmid   = %d
  name   = %q
  memory = 512
  cdrom {
    iso = "local-btrfs:iso/alpine-virt.iso"
  }
  boot_order = ["ide2"]
}

action "pve_vm_start" "a" {
  config {
    node = pve_vm.test.node
    vmid = pve_vm.test.vmid
  }
}

resource "terraform_data" "start_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_start.a]
    }
  }
}
%s
`, node, vmid, name, stop)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccVMGone(t, node, vmid),
		Steps: []resource.TestStep{
			{
				Config: config(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					vmRunning(true),
					testAccConfigHasCDROM(t, node, vmid),
				),
			},
			{
				Config: config(true),
				Check:  vmRunning(false),
			},
		},
	})
}

// testAccConfigHasCDROM asserts the cdrom block actually reached PVE —
// before the block engine, ide2 was accepted but never sent.
func testAccConfigHasCDROM(t *testing.T, node string, vmid int) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		payload, err := testAccClient(t).DoRequest(context.Background(), "GET",
			fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid), nil)
		if err != nil {
			return err
		}
		var env struct {
			Data struct {
				Ide2 string `json:"ide2"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &env); err != nil {
			return err
		}
		if !strings.Contains(env.Data.Ide2, "media=cdrom") {
			return fmt.Errorf("ide2 = %q, want media=cdrom", env.Data.Ide2)
		}
		return nil
	}
}

func TestAccPhase3VmSnapshotRollback(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-snap-%d", vmid)
	config := fmt.Sprintf(`
resource "pve_vm" "test" {
  node   = %q
  vmid   = %d
  name   = %q
  memory = 512
}

resource "pve_vm_snapshot" "snap" {
  node        = pve_vm.test.node
  vmid        = pve_vm.test.vmid
  snapname    = "tfacc"
  description = "acceptance snapshot"
}

action "pve_vm_snapshot_rollback" "rb" {
  config {
    node     = pve_vm.test.node
    vmid     = pve_vm.test.vmid
    snapname = pve_vm_snapshot.snap.snapname
  }
}

resource "terraform_data" "rollback_trigger" {
  depends_on = [pve_vm_snapshot.snap]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_snapshot_rollback.rb]
    }
  }
}
`, node, vmid, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccVMGone(t, node, vmid),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_vm_snapshot.snap", tfjsonpath.New("id"), knownvalue.StringExact(fmt.Sprintf("%s/%d/tfacc", node, vmid))),
				},
			},
		},
	})
}

func TestAccPhase3VmClone(t *testing.T) {
	node := testAccNode(t)
	srcVMID := testAccVMID()
	dstVMID := testAccVMID()
	config := fmt.Sprintf(`
resource "pve_vm" "source" {
  node   = %q
  vmid   = %d
  name   = "tfacc-clone-src-%d"
  memory = 512
}

resource "pve_vm" "cloned" {
  node              = %q
  vmid              = %d
  name              = "tfacc-clone-dst-%d"
  clone_source_vmid = pve_vm.source.vmid
}
`, node, srcVMID, srcVMID, node, dstVMID, dstVMID)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccVMGone(t, node, srcVMID),
			testAccVMGone(t, node, dstVMID),
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_vm.cloned", tfjsonpath.New("id"), knownvalue.StringExact(fmt.Sprintf("%s/%d", node, dstVMID))),
					statecheck.ExpectKnownValue("pve_vm.cloned", tfjsonpath.New("name"), knownvalue.StringExact(fmt.Sprintf("tfacc-clone-dst-%d", dstVMID))),
				},
			},
		},
	})
}
