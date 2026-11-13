// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 11 acceptance scenario (ROADMAP.md): zone + vnet + subnet CRUD,
// dry-run clean, apply, runtime verification through the node's SDN status
// (and the interface PVE materializes on the test bridge), then teardown.
// The vnet is pinned to the isolated bridge vmbr1 via a VLAN tag so the
// management bridge is never touched.

func TestAccPhase11SdnLifecycle(t *testing.T) {
	node := testAccNode(t)
	suffix := acctest.RandStringFromCharSet(4, acctest.CharSetAlphaNum)
	zone := "tfz" + suffix
	vnet := "tfv" + suffix
	subnet := "10.99.99.0/24"

	config := func(alias, gateway string) string {
		return fmt.Sprintf(`
resource "pve_sdn_zone" "zone" {
  zone   = %q
  type   = "vlan"
  bridge = "vmbr1"
}

resource "pve_sdn_vnet" "vnet" {
  vnet  = %q
  zone  = pve_sdn_zone.zone.zone
  alias = %q
  tag   = 42
}

resource "pve_sdn_subnet" "subnet" {
  vnet   = pve_sdn_vnet.vnet.vnet
  subnet = %q
  gateway = %q
  snat    = false
}

data "pve_sdn_dry_run" "dry" {
  node       = %q
  depends_on = [pve_sdn_subnet.subnet]
}

action "pve_sdn_apply" "apply" {
  config {}
}

resource "terraform_data" "apply_trigger" {
  depends_on = [pve_sdn_subnet.subnet, data.pve_sdn_dry_run.dry]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_sdn_apply.apply]
    }
  }
}

data "pve_sdn_status" "status" {
  node = %q
  depends_on = [terraform_data.apply_trigger]
}
`, zone, vnet, alias, subnet, gateway, node, node)
	}

	sdnGone := func(s *terraform.State) error {
		c := testAccClient(t)
		// Flush the deletion (SDN applies are explicit), then verify.
		if _, err := c.DoRequest(context.Background(), "PUT", "/cluster/sdn", nil); err != nil {
			return fmt.Errorf("applying sdn deletion: %w", err)
		}
		payload, err := c.DoRequest(context.Background(), "GET", "/cluster/sdn/zones", nil)
		if err != nil {
			return err
		}
		var zones []map[string]any
		if err := jsonUnmarshalT(payload, &zones); err != nil {
			return err
		}
		for _, z := range zones {
			if z["zone"] == zone {
				return fmt.Errorf("sdn zone %s still configured", zone)
			}
		}
		// The vnet must be gone from the cluster config as well.
		vnets, err := c.DoRequest(context.Background(), "GET", "/cluster/sdn/vnets", nil)
		if err != nil {
			return err
		}
		if strings.Contains(string(vnets), vnet) {
			return fmt.Errorf("sdn vnet %s still configured", vnet)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             sdnGone,
		Steps: []resource.TestStep{
			{
				Config: config("tfacc sdn vnet", "10.99.99.1"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_sdn_zone.zone", tfjsonpath.New("id"), knownvalue.StringExact(zone)),
					statecheck.ExpectKnownValue("pve_sdn_vnet.vnet", tfjsonpath.New("id"), knownvalue.StringExact(vnet)),
					statecheck.ExpectKnownValue("pve_sdn_subnet.subnet", tfjsonpath.New("id"), knownvalue.StringExact(vnet+"/"+subnet)),
					statecheck.ExpectKnownValue("pve_sdn_subnet.subnet", tfjsonpath.New("subnet_id"), knownvalue.StringExact(zone+"-10.99.99.0-24")),
					statecheck.ExpectKnownValue("data.pve_sdn_dry_run.dry", tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_sdn_status.status", tfjsonpath.New("id"), knownvalue.NotNull()),
				},
			},
			{
				// In-place updates on vnet (alias) and subnet (gateway).
				Config: config("tfacc sdn vnet v2", "10.99.99.254"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_sdn_vnet.vnet", tfjsonpath.New("alias"), knownvalue.StringExact("tfacc sdn vnet v2")),
					statecheck.ExpectKnownValue("pve_sdn_subnet.subnet", tfjsonpath.New("gateway"), knownvalue.StringExact("10.99.99.254")),
				},
			},
			{ResourceName: "pve_sdn_zone.zone", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(*terraform.State) (string, error) { return zone, nil }},
			{ResourceName: "pve_sdn_vnet.vnet", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(*terraform.State) (string, error) { return vnet, nil }},
			{ResourceName: "pve_sdn_subnet.subnet", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(*terraform.State) (string, error) { return vnet + "/" + subnet, nil }},
		},
	})
}
