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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 6 acceptance scenarios (ROADMAP.md), scoped to additive/safe
// operations on the live node: the staged-apply network lifecycle on a
// dummy bridge (never the management interface), same-value singleton
// round-trips for DNS and time, and read-only service/APT data sources.
// Disk management (no scratch disk), subscription writes, and node power
// actions are deliberately not exercised.

func TestAccPhase6NetworkStagedApply(t *testing.T) {
	node := testAccNode(t)
	iface := "vmbr99"
	config := func(vids string) string {
		return fmt.Sprintf(`
resource "pve_node_network" "test" {
  node              = %q
  iface             = %q
  type              = "bridge"
  bridge_vlan_aware = true
  bridge_vids       = %q
  autostart         = true
}
`, node, iface, vids)
	}
	// ifaceGone: the staged deletion must be flushed (apply) before the
	// interface disappears from PVE's applied state.
	ifaceGone := func(s *terraform.State) error {
		c := testAccClient(t)
		if _, err := c.DoRequest(context.Background(), "PUT", "/nodes/"+node+"/network", nil); err != nil {
			return fmt.Errorf("flushing staged network deletion: %w", err)
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
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             ifaceGone,
		Steps: []resource.TestStep{
			{
				// Create stages the bridge (PVE semantics: no apply until
				// an update runs).
				Config: config("9"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("id"), knownvalue.StringExact(node+"/"+iface)),
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("type"), knownvalue.StringExact("bridge")),
				},
			},
			{
				// Update re-writes the interface and applies all pending
				// changes.
				Config: config("9-10"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_node_network.test", tfjsonpath.New("bridge_vids"), knownvalue.StringExact("9-10")),
				},
			},
			{
				ResourceName:      "pve_node_network.test",
				ImportState:       true,
				ImportStateVerify: true,
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
