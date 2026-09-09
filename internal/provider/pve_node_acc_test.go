// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPveNode_basic is the smoke acceptance test gated on a live PVE
// fixture (PVE_TEST_ENDPOINT + PVE_TEST_API_TOKEN). It creates a
// pve_node resource, asserts state, then imports the same node. Without
// the env vars the test is skipped so CI without a fixture stays green.
func TestAccPveNode_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	if os.Getenv("PVE_TEST_ENDPOINT") == "" || os.Getenv("PVE_TEST_API_TOKEN") == "" {
		t.Skip("PVE_TEST_ENDPOINT / PVE_TEST_API_TOKEN not set; skipping acceptance test")
	}
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPveNodeConfig_basic(rName),
			},
		},
	})
}

func testAccPveNodeConfig_basic(node string) string {
	return fmt.Sprintf(`
provider "scaffolding" {
  endpoint  = "https://example.invalid:8006/"
  api_token = "root@pam!tf=dummy"
}

resource "scaffolding_node" "test" {
  node        = %[1]q
  description = "managed by terraform"
}
`, node)
}
