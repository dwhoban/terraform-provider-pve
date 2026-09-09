// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveSecurityGroupFirewallRules_KeysRequireReplacement pins the key
// attribute semantics: group is a required string that forces
// replacement.
func TestPveSecurityGroupFirewallRules_KeysRequireReplacement(t *testing.T) {
	r := NewPveSecurityGroupFirewallRulesResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	group, ok := schemaResp.Schema.Attributes["group"].(schema.StringAttribute)
	if !ok {
		t.Fatal("group attribute is not a StringAttribute")
	}
	if !group.Required {
		t.Fatal("group attribute should be Required")
	}
	if len(group.PlanModifiers) == 0 {
		t.Fatal("group attribute should force replacement on change")
	}
}
