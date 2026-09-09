// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveVnetFirewallRules_KeysRequireReplacement pins the key
// attribute semantics: vnet is a required string that forces
// replacement.
func TestPveVnetFirewallRules_KeysRequireReplacement(t *testing.T) {
	r := NewPveVnetFirewallRulesResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	vnet, ok := schemaResp.Schema.Attributes["vnet"].(schema.StringAttribute)
	if !ok {
		t.Fatal("vnet attribute is not a StringAttribute")
	}
	if !vnet.Required {
		t.Fatal("vnet attribute should be Required")
	}
	if len(vnet.PlanModifiers) == 0 {
		t.Fatal("vnet attribute should force replacement on change")
	}
}
