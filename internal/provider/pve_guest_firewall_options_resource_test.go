// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// guestFirewallOptionsAttrTypes maps the resource schema to Terraform
// types.
func guestFirewallOptionsAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":            tftypes.String,
		"node":          tftypes.String,
		"guest_type":    tftypes.String,
		"vmid":          tftypes.Number,
		"enable":        tftypes.Bool,
		"dhcp":          tftypes.Bool,
		"ipfilter":      tftypes.Bool,
		"macfilter":     tftypes.Bool,
		"ndp":           tftypes.Bool,
		"radv":          tftypes.Bool,
		"log_level_in":  tftypes.String,
		"log_level_out": tftypes.String,
		"policy_in":     tftypes.String,
		"policy_out":    tftypes.String,
	}
}

// TestPveGuestFirewallOptionsResource_MetadataAndSchema covers the
// resource's type name, the key attributes, and closed-set descriptions.
func TestPveGuestFirewallOptionsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveGuestFirewallOptionsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestFirewallOptions)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "guest_type", "vmid", "enable", "dhcp", "ipfilter", "macfilter", "ndp", "radv", "log_level_in", "log_level_out", "policy_in", "policy_out"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"node", "guest_type", "vmid"} {
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
	id, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !id.IsComputed() || id.IsOptional() {
		t.Fatal("id must be computed-only (singleton semantics)")
	}
	guestType, ok := schemaResp.Schema.Attributes["guest_type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("guest_type attribute is not a StringAttribute")
	}
	if !strings.Contains(guestType.MarkdownDescription, "`qemu`") || !strings.Contains(guestType.MarkdownDescription, "`lxc`") {
		t.Fatalf("guest_type description must enumerate the closed set: %q", guestType.MarkdownDescription)
	}
	vmid, ok := schemaResp.Schema.Attributes["vmid"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("vmid attribute is not an Int64Attribute")
	}
	if !strings.Contains(vmid.MarkdownDescription, "Must be between 100 and 999999999") {
		t.Fatalf("vmid description must state the range: %q", vmid.MarkdownDescription)
	}
}

// TestPveGuestFirewallOptionsParseImportID covers the import ID grammar
// `<node>:<guest_type>:<vmid>` and its error cases.
func TestPveGuestFirewallOptionsParseImportID(t *testing.T) {
	for _, tc := range []struct {
		id        string
		node      string
		guestType string
		vmid      int64
		wantError bool
	}{
		{id: "pve1:qemu:100", node: "pve1", guestType: "qemu", vmid: 100},
		{id: "pve1:lxc:101", node: "pve1", guestType: "lxc", vmid: 101},
		{id: "pve1:qemu", wantError: true},
		{id: "pve1::100", wantError: true},
		{id: ":qemu:100", wantError: true},
		{id: "pve1:qemu:abc", wantError: true},
		{id: "pve1:qemu:100:extra", wantError: true},
	} {
		node, guestType, vmid, err := guestFirewallOptionsParseImportID(tc.id)
		if tc.wantError {
			if err == nil {
				t.Fatalf("id %q: expected error", tc.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("id %q: %v", tc.id, err)
		}
		if node != tc.node || guestType != tc.guestType || vmid != tc.vmid {
			t.Fatalf("id %q: got (%s,%s,%d), want (%s,%s,%d)", tc.id, node, guestType, vmid, tc.node, tc.guestType, tc.vmid)
		}
	}
}

// TestPveGuestFirewallOptionsResource_CreateAndUpdate runs create and
// update against a fake API, asserting the qemu path and the node/vmid
// keys in the body.
func TestPveGuestFirewallOptionsResource_CreateAndUpdate(t *testing.T) {
	r := NewPveGuestFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveGuestFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var lastPath string
	var lastBody []byte
	var lastQuery string
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/nodes/pve1/qemu/100/firewall/options":
			lastPath = req.URL.Path
			lastQuery = req.URL.RawQuery
			lastBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/qemu/100/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"enable":1,"dhcp":0,"ipfilter":1,"macfilter":1,"ndp":1,"radv":0,"log_level_in":"emerg","policy_in":"DROP","policy_out":"REJECT"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	attrTypes := guestFirewallOptionsAttrTypes()
	newVal := func(vals map[string]tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
	}
	set := func(overrides map[string]tftypes.Value) map[string]tftypes.Value {
		vals := make(map[string]tftypes.Value, len(attrTypes))
		for name, at := range attrTypes {
			vals[name] = tftypes.NewValue(at, nil)
		}
		for name, v := range overrides {
			vals[name] = v
		}
		return vals
	}
	createVals := set(map[string]tftypes.Value{
		"node":       tftypes.NewValue(tftypes.String, "pve1"),
		"guest_type": tftypes.NewValue(tftypes.String, "qemu"),
		"vmid":       tftypes.NewValue(tftypes.Number, 100),
		"enable":     tftypes.NewValue(tftypes.Bool, true),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: newVal(set(nil))},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(createVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(createVals)},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveGuestFirewallOptionsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "pve1:qemu:100" || !created.Enable.ValueBool() || created.MacFilter.ValueBool() != true || created.Radv.ValueBool() {
		t.Fatalf("created state = %+v", created)
	}
	if !strings.Contains(string(lastBody), `"node":"pve1"`) || !strings.Contains(string(lastBody), `"vmid":100`) {
		t.Fatalf("create body missing node/vmid keys: %q", lastBody)
	}

	// Update: dhcp set in state but cleared in the plan must travel in the
	// delete query.
	updateVals := set(map[string]tftypes.Value{
		"node":       tftypes.NewValue(tftypes.String, "pve1"),
		"guest_type": tftypes.NewValue(tftypes.String, "qemu"),
		"vmid":       tftypes.NewValue(tftypes.Number, 100),
		"enable":     tftypes.NewValue(tftypes.Bool, true),
	})
	stateVals := set(map[string]tftypes.Value{
		"node":       tftypes.NewValue(tftypes.String, "pve1"),
		"guest_type": tftypes.NewValue(tftypes.String, "qemu"),
		"vmid":       tftypes.NewValue(tftypes.Number, 100),
		"enable":     tftypes.NewValue(tftypes.Bool, true),
		"dhcp":       tftypes.NewValue(tftypes.Bool, true),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: newVal(stateVals)},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if lastPath != "/nodes/pve1/qemu/100/firewall/options" {
		t.Fatalf("update path = %q", lastPath)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "dhcp") {
		t.Fatalf("update query = %q", lastQuery)
	}
}

// TestPveGuestFirewallOptionsResource_Read404Removes verifies Read drops
// the singleton from state when the guest vanished out of band.
func TestPveGuestFirewallOptionsResource_Read404Removes(t *testing.T) {
	r := NewPveGuestFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveGuestFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such vm"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrTypes := guestFirewallOptionsAttrTypes()
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	vals["node"] = tftypes.NewValue(tftypes.String, "pve1")
	vals["guest_type"] = tftypes.NewValue(tftypes.String, "qemu")
	vals["vmid"] = tftypes.NewValue(tftypes.Number, 100)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
