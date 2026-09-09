// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveGuestFirewallRules_KeysRequireReplacement pins the key
// attribute semantics: node and guest_type are required strings and
// vmid a required int64, all forcing replacement; vmid carries the
// pin's range validator.
func TestPveGuestFirewallRules_KeysRequireReplacement(t *testing.T) {
	r := NewPveGuestFirewallRulesResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	guestType, ok := schemaResp.Schema.Attributes["guest_type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("guest_type attribute is not a StringAttribute")
	}
	if !guestType.Required || len(guestType.PlanModifiers) == 0 {
		t.Fatal("guest_type should be Required and force replacement")
	}
	if len(guestType.Validators) == 0 {
		t.Fatal("guest_type should carry the qemu|lxc OneOf validator")
	}
	vmid, ok := schemaResp.Schema.Attributes["vmid"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("vmid attribute is not an Int64Attribute")
	}
	if !vmid.Required || len(vmid.PlanModifiers) == 0 {
		t.Fatal("vmid should be Required and force replacement")
	}
	if len(vmid.Validators) == 0 {
		t.Fatal("vmid should carry the pin range validator")
	}
}
