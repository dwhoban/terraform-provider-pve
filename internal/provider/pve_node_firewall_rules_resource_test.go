// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveNodeFirewallRules_KeysRequireReplacement pins the key
// attribute semantics: node is a required string that forces
// replacement.
func TestPveNodeFirewallRules_KeysRequireReplacement(t *testing.T) {
	r := NewPveNodeFirewallRulesResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	node, ok := schemaResp.Schema.Attributes["node"].(schema.StringAttribute)
	if !ok {
		t.Fatal("node attribute is not a StringAttribute")
	}
	if !node.Required {
		t.Fatal("node attribute should be Required")
	}
	if len(node.PlanModifiers) == 0 {
		t.Fatal("node attribute should force replacement on change")
	}
}
