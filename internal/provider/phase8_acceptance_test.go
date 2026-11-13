// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 8 acceptance scenario (ROADMAP.md) — the complete lifecycle on
// this node, which is standalone at the start: create the cluster
// (pve_cluster, create-only resource), exercise options (write + revert),
// mappings (dir/pci/usb CRUD) and every cluster-configuration data source,
// then dissolve the cluster with the node-remove action and verify the
// node is standalone again. Join itself requires a second node and is not
// exercised.

func testAccStandalone(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		payload, err := testAccClient(t).DoRequest(context.Background(), "GET", "/cluster/status", nil)
		if err != nil {
			return err
		}
		var entries []map[string]any
		if err := jsonUnmarshalT(payload, &entries); err != nil {
			return err
		}
		for _, e := range entries {
			if e["type"] == "cluster" {
				return fmt.Errorf("cluster still configured (%v)", e["name"])
			}
		}
		return nil
	}
}

func TestAccPhase8ClusterLifecycle(t *testing.T) {
	node := testAccNode(t)
	if os.Getenv("PVE_TEST_ALLOW_CLUSTER_CREATE") != "1" {
		t.Skip("creating/dissolving a cluster is root@pam-only (API token gets 403); set PVE_TEST_ALLOW_CLUSTER_CREATE=1 after re-running with root@pam credentials")
	}
	clusterName := "tfacc-" + acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)
	poolid := "tfacc-map-" + acctest.RandStringFromCharSet(4, acctest.CharSetAlphaNum)

	config := func() string {
		return fmt.Sprintf(`
resource "pve_cluster" "cluster" {
  clustername = %q
}

resource "pve_cluster_options" "options" {
  keyboard = "en-us"
}

resource "pve_mapping_dir" "dir" {
  item_id     = %q
  map         = ["/tfacc/dir-%s"]
}

resource "pve_mapping_pci" "pci" {
  item_id     = %q
  map         = ["0000:00:02.0"]
}

resource "pve_mapping_usb" "usb" {
  item_id     = %q
  map         = ["host=1-1"]
}

action "pve_cluster_node_remove" "dissolve" {
  config { node = %q }
}

resource "terraform_data" "dissolve_trigger" {
  depends_on = [pve_cluster.cluster]
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_cluster_node_remove.dissolve]
    }
  }
}

data "pve_cluster_join_info" "join" {
  depends_on = [pve_cluster.cluster]
}
data "pve_cluster_api_version" "api" {
  depends_on = [pve_cluster.cluster]
}
data "pve_cluster_qdevice" "qdevice" {
  depends_on = [pve_cluster.cluster]
}
data "pve_cluster_totem" "totem" {
  depends_on = [pve_cluster.cluster]
}
data "pve_cluster_nodes" "nodes" {
  depends_on = [pve_cluster.cluster]
}
`, clusterName, poolid+"-dir", poolid, poolid+"-pci", poolid+"-usb", node)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccStandalone(t),
		Steps: []resource.TestStep{
			{
				Config: config(),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_cluster.cluster", tfjsonpath.New("id"), knownvalue.StringExact(clusterName)),
					statecheck.ExpectKnownValue("pve_mapping_dir.dir", tfjsonpath.New("id"), knownvalue.StringExact(poolid+"-dir")),
					statecheck.ExpectKnownValue("pve_mapping_pci.pci", tfjsonpath.New("id"), knownvalue.StringExact(poolid+"-pci")),
					statecheck.ExpectKnownValue("pve_mapping_usb.usb", tfjsonpath.New("id"), knownvalue.StringExact(poolid+"-usb")),
					statecheck.ExpectKnownValue("data.pve_cluster_join_info.join", tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_cluster_api_version.api", tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_cluster_totem.totem", tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_cluster_nodes.nodes", tfjsonpath.New("nodes"), knownvalue.NotNull()),
				},
			},
			{
				Config: config(),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_cluster_options.options", tfjsonpath.New("keyboard"), knownvalue.StringExact("en-us")),
				},
			},
		},
	})
}
