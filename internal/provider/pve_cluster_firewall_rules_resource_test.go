// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// firewallRulesRuleAttrTypes returns the nested rule object's Terraform
// types, shared by the rules-list raw values in these tests.
func firewallRulesRuleAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"pos":       tftypes.Number,
		"enable":    tftypes.Bool,
		"type":      tftypes.String,
		"action":    tftypes.String,
		"macro":     tftypes.String,
		"proto":     tftypes.String,
		"dport":     tftypes.String,
		"sport":     tftypes.String,
		"source":    tftypes.String,
		"dest":      tftypes.String,
		"icmp_type": tftypes.String,
		"iface":     tftypes.String,
		"log":       tftypes.String,
		"comment":   tftypes.String,
		"ipversion": tftypes.Number,
	}
}

// firewallRulesTestRuleRaw renders one rule object raw value.
func firewallRulesTestRuleRaw(enable bool, rtype, action string) tftypes.Value {
	vals := map[string]tftypes.Value{
		"pos":       tftypes.NewValue(tftypes.Number, nil),
		"enable":    tftypes.NewValue(tftypes.Bool, enable),
		"type":      tftypes.NewValue(tftypes.String, rtype),
		"action":    tftypes.NewValue(tftypes.String, action),
		"macro":     tftypes.NewValue(tftypes.String, nil),
		"proto":     tftypes.NewValue(tftypes.String, nil),
		"dport":     tftypes.NewValue(tftypes.String, nil),
		"sport":     tftypes.NewValue(tftypes.String, nil),
		"source":    tftypes.NewValue(tftypes.String, nil),
		"dest":      tftypes.NewValue(tftypes.String, nil),
		"icmp_type": tftypes.NewValue(tftypes.String, nil),
		"iface":     tftypes.NewValue(tftypes.String, nil),
		"log":       tftypes.NewValue(tftypes.String, nil),
		"comment":   tftypes.NewValue(tftypes.String, nil),
		"ipversion": tftypes.NewValue(tftypes.Number, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: firewallRulesRuleAttrTypes()}, vals)
}

// firewallRulesClusterRaw renders the cluster resource raw value.
func firewallRulesClusterRaw(rules []tftypes.Value) tftypes.Value {
	attrTypes := map[string]tftypes.Type{
		"id":    tftypes.String,
		"rules": tftypes.List{ElementType: tftypes.Object{AttributeTypes: firewallRulesRuleAttrTypes()}},
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":    tftypes.NewValue(tftypes.String, nil),
		"rules": tftypes.NewValue(attrTypes["rules"], rules),
	})
}

// TestPveClusterFirewallRules_ResourceLifecycle runs the full create,
// read, update, and delete paths of the cluster ruleset resource against
// a stable fake ruleset: create POSTs in order, update converges via
// in-place PUT plus append, and delete unwinds highest position first.
func TestPveClusterFirewallRules_ResourceLifecycle(t *testing.T) {
	srv := &firewallRulesFakeServer{
		t:        t,
		basePath: "/cluster/firewall/rules",
	}
	r := NewPveClusterFirewallRulesResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveClusterFirewallRulesResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = firewallRulesTestClient(t, srv.handler)
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	newRequest := func(raw tftypes.Value) resource.CreateRequest {
		return resource.CreateRequest{
			Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
			Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
		}
	}
	twoRules := []tftypes.Value{
		firewallRulesTestRuleRaw(true, "in", "ACCEPT"),
		firewallRulesTestRuleRaw(true, "in", "DROP"),
	}

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, newRequest(firewallRulesClusterRaw(twoRules)), createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveClusterFirewallRulesResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != pveClusterFirewallRulesID {
		t.Fatalf("created id = %q", created.ID.ValueString())
	}
	if len(created.Rules) != 2 || !created.Rules[0].Enable.ValueBool() {
		t.Fatalf("created rules = %+v", created.Rules)
	}
	if created.Rules[0].Pos.ValueInt64() != 0 || created.Rules[1].Pos.ValueInt64() != 1 {
		t.Fatalf("created positions = %d,%d", created.Rules[0].Pos.ValueInt64(), created.Rules[1].Pos.ValueInt64())
	}

	// Update: change the second rule in place and append a third.
	threeRules := append(append([]tftypes.Value{}, twoRules[0]),
		firewallRulesTestRuleRaw(false, "out", "REJECT"),
		firewallRulesTestRuleRaw(true, "in", "ACCEPT"),
	)
	updateReq := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: firewallRulesClusterRaw(threeRules)},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: updateReq.State.Raw}}
	r.Update(ctx, updateReq, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	var updated pveClusterFirewallRulesResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if len(updated.Rules) != 3 || updated.Rules[1].Type.ValueString() != "out" || updated.Rules[1].Action.ValueString() != "REJECT" {
		t.Fatalf("updated rules = %+v", updated.Rules)
	}

	// Delete removes every rule, highest position first.
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: updateResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
	if len(srv.rules) != 0 {
		t.Fatalf("ruleset not empty after delete: %+v", srv.rules)
	}

	// Read after teardown reports the now-empty ruleset without error.
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: updateResp.State.Raw}}
	r.Read(ctx, resource.ReadRequest{State: readResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var afterRead pveClusterFirewallRulesResourceModel
	if err := readResp.State.Get(ctx, &afterRead); err != nil {
		t.Fatalf("State.Get after read: %v", err)
	}
	if len(afterRead.Rules) != 0 {
		t.Fatalf("rules after emptying read = %+v", afterRead.Rules)
	}
}
