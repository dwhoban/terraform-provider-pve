// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 4 acceptance scenarios (ROADMAP.md): container lifecycle (create
// from template, config update, import), start/stop actions, and snapshot
// lifecycle. Guests are scratch Alpine LXC containers with ephemeral
// rootfs on local-btrfs, destroyed afterwards.

const testAccCTTemplate = "local-btrfs:vztmpl/alpine-3.22-default_20250617_amd64.tar.xz"

func testAccCTGone(t *testing.T, node string, vmid int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid)
		_, err := testAccClient(t).DoRequest(context.Background(), "GET", path, nil)
		if err == nil {
			return fmt.Errorf("container %d still exists", vmid)
		}
		if !pveclient.IsNotFound(err) {
			return err
		}
		return nil
	}
}

func TestAccPhase4ContainerLifecycle(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-ct-%d", vmid)
	config := func(memory int) string {
		return fmt.Sprintf(`
resource "pve_container" "test" {
  node       = %q
  vmid       = %d
  ostemplate = %q
  hostname   = %q
  storage    = "local-btrfs"
  memory     = %d
  cores      = 1
  # rootfs deliberately unset: PVE normalizes it to the concrete volume
  # path, which can't be stated in config at create time.
}
`, node, vmid, testAccCTTemplate, name, memory)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCTGone(t, node, vmid),
		Steps: []resource.TestStep{
			{
				Config: config(256),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_container.test", tfjsonpath.New("id"), knownvalue.StringExact(fmt.Sprintf("%s/%d", node, vmid))),
					statecheck.ExpectKnownValue("pve_container.test", tfjsonpath.New("hostname"), knownvalue.StringExact(name)),
				},
			},
			{
				Config: config(512),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_container.test", tfjsonpath.New("memory"), knownvalue.Int64Exact(512)),
				},
			},
			{
				ResourceName:            "pve_container.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ostemplate", "storage", "rootfs", "swap", "arch", "ostype", "unprivileged"}, // create-only or server-defaulted
			},
		},
	})
}

func TestAccPhase4ContainerStartStop(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-ctrun-%d", vmid)
	ctRunning := func(want string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			payload, err := testAccClient(t).DoRequest(context.Background(), "GET",
				fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid), nil)
			if err != nil {
				return err
			}
			var st struct {
				Status string `json:"status"`
			}
			if err := jsonUnmarshalT(payload, &st); err != nil {
				return err
			}
			if st.Status != want {
				return fmt.Errorf("container status = %q, want %q", st.Status, want)
			}
			return nil
		}
	}
	base := fmt.Sprintf(`
resource "pve_container" "test" {
  node       = %q
  vmid       = %d
  ostemplate = %q
  hostname   = %q
  storage    = "local-btrfs"
  memory     = 256
  cores      = 1
}
`, node, vmid, testAccCTTemplate, name)
	start := fmt.Sprintf(`%s
action "pve_container_start" "a" {
  config {
    node = pve_container.test.node
    vmid = pve_container.test.vmid
  }
}

resource "terraform_data" "start_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_container_start.a]
    }
  }
}
`, base)
	stop := fmt.Sprintf(`%s
action "pve_container_stop" "s" {
  config {
    node = pve_container.test.node
    vmid = pve_container.test.vmid
  }
}

resource "terraform_data" "stop_trigger" {
  depends_on = [terraform_data.start_trigger]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_container_stop.s]
    }
  }
}
`, start)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCTGone(t, node, vmid),
		Steps: []resource.TestStep{
			{Config: start, Check: ctRunning("running")},
			{Config: stop, Check: ctRunning("stopped")},
		},
	})
}

func TestAccPhase4ContainerSnapshot(t *testing.T) {
	node := testAccNode(t)
	vmid := testAccVMID()
	name := fmt.Sprintf("tfacc-ctsnap-%d", vmid)
	config := fmt.Sprintf(`
resource "pve_container" "test" {
  node       = %q
  vmid       = %d
  ostemplate = %q
  hostname   = %q
  storage    = "local-btrfs"
  memory     = 256
  cores      = 1
}

resource "pve_container_snapshot" "snap" {
  node     = pve_container.test.node
  vmid     = pve_container.test.vmid
  snapname = "tfacc"
}
`, node, vmid, testAccCTTemplate, name)
	_ = acctest.RandStringFromCharSet(4, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCTGone(t, node, vmid),
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_container_snapshot.snap", tfjsonpath.New("id"), knownvalue.StringExact(fmt.Sprintf("%s/%d/tfacc", node, vmid))),
				},
			},
		},
	})
}
