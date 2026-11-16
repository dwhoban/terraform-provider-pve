// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 6 acceptance scenarios (ROADMAP.md), scoped to additive/safe
// operations on the live node: the pending-change network lifecycle (apply action and apply_on_change) on a
// dummy bridge (never the management interface), same-value singleton
// round-trips for DNS and time, and read-only service/APT data sources.
// Disk management (no scratch disk), subscription writes, and node power
// actions are deliberately not exercised.

func TestAccPhase6NetworkPendingApply(t *testing.T) {
	node := testAccNode(t)
	iface := "vmbr99"
	config := func(vids string) string {
		return fmt.Sprintf(`
resource "pve_node_network" "test" {
  node              = %[1]q
  iface             = %[2]q
  type              = "bridge"
  bridge_vlan_aware = true
  bridge_vids       = %[3]q
  autostart         = true
}

action "pve_node_network_apply" "apply" {
  config {
    node = %[1]q
  }
}

resource "terraform_data" "apply_trigger" {
  input = pve_node_network.test
  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.pve_node_network_apply.apply]
    }
  }
}
`, node, iface, vids)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccNetworkIfaceGone(t, node, iface),
		Steps: []resource.TestStep{
			{
				// Create records the bridge as a pending change; the
				// action trigger commits it.
				Config: config("9"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("id"), knownvalue.StringExact(node+"/"+iface)),
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("type"), knownvalue.StringExact("bridge")),
				},
				Check: testAccNetworkIfaceActive(t, node, iface),
			},
			{
				// Update records another pending change; the action
				// trigger commits it.
				Config: config("9-10"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("bridge_vids"), knownvalue.StringExact("9-10")),
				},
				Check: testAccNetworkIfaceActive(t, node, iface),
			},
			{
				ResourceName:      "pve_node_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccNetworkIfaceActive requires the interface to be active on the node
// (GET /nodes/{node}/network reports active=1), i.e. its pending change was
// applied.
func testAccNetworkIfaceActive(t *testing.T, node, iface string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/nodes/"+node+"/network", nil)
		if err != nil {
			return err
		}
		var list []map[string]any
		if err := jsonUnmarshalT(payload, &list); err != nil {
			return err
		}
		for _, e := range list {
			if v, _ := e["iface"].(string); v != iface {
				continue
			}
			switch a := e["active"].(type) {
			case bool:
				if a {
					return nil
				}
			case float64:
				if a == 1 {
					return nil
				}
			}
			return fmt.Errorf("interface %s is configured but not active (active=%v)", iface, e["active"])
		}
		return fmt.Errorf("interface %s not found on node %s", iface, node)
	}
}

// testAccNetworkIfaceGone checks the interface is gone after destroy:
// destroy only records a pending removal; flush it (apply) before verifying.
func testAccNetworkIfaceGone(t *testing.T, node, iface string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		if _, err := c.DoRequest(context.Background(), "PUT", "/nodes/"+node+"/network", nil); err != nil {
			return fmt.Errorf("flushing pending network deletion: %w", err)
		}
		payload, err := c.DoRequest(context.Background(), "GET", "/nodes/"+node+"/network", nil)
		if err != nil {
			return err
		}
		var list []map[string]any
		if err := jsonUnmarshalT(payload, &list); err != nil {
			return err
		}
		for _, e := range list {
			if v, _ := e["iface"].(string); v == iface {
				return fmt.Errorf("interface %s still configured", iface)
			}
		}
		return nil
	}
}

// Dropping cidr makes PVE rewrite method to manual; the plan must carry
// that value instead of (known after apply), and apply must agree.
func TestAccPhase6NetworkMethodKnownInPlan(t *testing.T) {
	node := testAccNode(t)
	config := func(cidr string) string {
		return fmt.Sprintf(`
resource "pve_node_network" "test" {
  node            = %q
  iface           = "vmbr98"
  type            = "bridge"
  autostart       = true
  apply_on_change = true
%s}
`, node, cidr)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccNetworkIfaceGone(t, node, "vmbr98"),
		Steps: []resource.TestStep{
			{
				Config: config(`  cidr            = "192.0.2.10/24"` + "\n"), // TEST-NET-1: never routed
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("method"), knownvalue.StringExact("static")),
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("apply_on_change"), knownvalue.Bool(true)),
				},
				Check: testAccNetworkIfaceActive(t, node, "vmbr98"),
			},
			{
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pve_node_network.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("method"), knownvalue.StringExact("manual")),
						plancheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("method6"), knownvalue.StringExact("manual")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("method"), knownvalue.StringExact("manual")),
				},
			},
		},
	})
}

func TestAccPhase6DnsSingletonRoundTrip(t *testing.T) {
	node := testAccNode(t)
	// Write the CURRENT values back: exercises singleton create/update/
	// read against the live node without changing resolver behavior.
	config := fmt.Sprintf(`
resource "pve_node_dns" "test" {
  node   = %q
  search = "bsknhbn.cloud"
  dns1   = "10.0.10.1"
}
`, node)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_dns.test", tfjsonpath.New("search"), knownvalue.StringExact("bsknhbn.cloud")),
					statecheck.ExpectKnownValue("pve_node_dns.test", tfjsonpath.New("dns1"), knownvalue.StringExact("10.0.10.1")),
				},
			},
		},
	})
}

func TestAccPhase6TimeSingletonRoundTrip(t *testing.T) {
	node := testAccNode(t)
	config := fmt.Sprintf(`
resource "pve_node_time" "test" {
  node     = %q
  timezone = "Australia/Brisbane"
}
`, node)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_time.test", tfjsonpath.New("timezone"), knownvalue.StringExact("Australia/Brisbane")),
				},
			},
		},
	})
}

func TestAccPhase6ReadonlyDataSources(t *testing.T) {
	node := testAccNode(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "pve_node_services" "test" { node = %q }
data "pve_apt" "test" { node = %q }
data "pve_apt_versions" "versions" { node = %q }
`, node, node, node),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.pve_node_services.test", tfjsonpath.New("services"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_apt.test", tfjsonpath.New("entries"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_apt_versions.versions", tfjsonpath.New("id"), knownvalue.NotNull()),
				},
			},
		},
	})
}

func jsonUnmarshalT(payload []byte, v any) error { return json.Unmarshal(payload, v) }
