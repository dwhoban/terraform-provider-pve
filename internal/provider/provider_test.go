// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	tfprotov6 "github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"pve": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv(pveclient.EnvAPIURL) == "" || os.Getenv(pveclient.EnvAPIToken) == "" {
		t.Fatalf("acceptance tests require %s and %s to be set", pveclient.EnvAPIURL, pveclient.EnvAPIToken)
	}
}

func testAccPreCheckURL(t *testing.T) {
	t.Helper()
	if os.Getenv(pveclient.EnvAPIURL) == "" {
		t.Fatalf("acceptance tests require %s to be set", pveclient.EnvAPIURL)
	}
}

// TestAccPveProvider_invalidCredentials pins the plan-time diagnostic for a
// rejected token, in both the combined api_token form and the split
// api_token_id/api_secret form: configure-time validation must fail the
// operation with an actionable error naming the credential source, not a
// raw API error.
func TestAccPveProvider_invalidCredentials(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckURL(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pve" {
  api_token = "root@pam!nonexistent=00000000-0000-0000-0000-000000000000"
}

data "pve_version" "current" {}
`,
				ExpectError: regexp.MustCompile(`Invalid Proxmox VE Credentials`),
			},
			{
				Config: `
provider "pve" {
  api_token_id = "root@pam!nonexistent"
  api_secret   = "00000000-0000-0000-0000-000000000000"
}

data "pve_version" "current" {}
`,
				ExpectError: regexp.MustCompile(`Invalid Proxmox VE Credentials`),
			},
		},
	})
}
