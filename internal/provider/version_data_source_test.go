// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccPveVersionDataSource_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVersionDataSourceConfig_basic,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.pve_version.current", tfjsonpath.New("version"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_version.current", tfjsonpath.New("release"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.pve_version.current", tfjsonpath.New("repoid"), knownvalue.NotNull()),
				},
			},
		},
	})
}

const testAccVersionDataSourceConfig_basic = `
data "pve_version" "current" {}
`
