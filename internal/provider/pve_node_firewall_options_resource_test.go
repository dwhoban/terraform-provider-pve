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

// nodeFirewallOptionsAttrTypes maps the resource schema to Terraform types.
func nodeFirewallOptionsAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":                                   tftypes.String,
		"node":                                 tftypes.String,
		"enable":                               tftypes.Bool,
		"log_level_in":                         tftypes.String,
		"log_level_out":                        tftypes.String,
		"log_level_forward":                    tftypes.String,
		"log_nf_conntrack":                     tftypes.Bool,
		"ndp":                                  tftypes.Bool,
		"nf_conntrack_allow_invalid":           tftypes.Bool,
		"nf_conntrack_helpers":                 tftypes.String,
		"nf_conntrack_max":                     tftypes.Number,
		"nf_conntrack_tcp_timeout_established": tftypes.Number,
		"nf_conntrack_tcp_timeout_syn_recv":    tftypes.Number,
		"nftables":                             tftypes.Bool,
		"nosmurfs":                             tftypes.Bool,
		"protection_synflood":                  tftypes.Bool,
		"protection_synflood_burst":            tftypes.Number,
		"protection_synflood_rate":             tftypes.Number,
		"smurf_log_level":                      tftypes.String,
		"tcpflags":                             tftypes.Bool,
		"tcp_flags_log_level":                  tftypes.String,
	}
}

// TestPveNodeFirewallOptionsResource_MetadataAndSchema covers the
// resource's type name, the node key, and closed-set/range descriptions.
func TestPveNodeFirewallOptionsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveNodeFirewallOptionsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeFirewallOptions)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "enable", "log_level_in", "log_level_out", "log_level_forward", "log_nf_conntrack", "ndp", "nf_conntrack_allow_invalid", "nf_conntrack_helpers", "nf_conntrack_max", "nf_conntrack_tcp_timeout_established", "nf_conntrack_tcp_timeout_syn_recv", "nftables", "nosmurfs", "protection_synflood", "protection_synflood_burst", "protection_synflood_rate", "smurf_log_level", "tcpflags", "tcp_flags_log_level"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	id, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !id.IsComputed() || id.IsOptional() {
		t.Fatal("id must be computed-only (singleton semantics)")
	}
	logLevelIn, ok := schemaResp.Schema.Attributes["log_level_in"].(schema.StringAttribute)
	if !ok {
		t.Fatal("log_level_in attribute is not a StringAttribute")
	}
	if !strings.Contains(logLevelIn.MarkdownDescription, "`nolog`") || !strings.Contains(logLevelIn.MarkdownDescription, "Must be one of") {
		t.Fatalf("log_level_in description must enumerate the closed set: %q", logLevelIn.MarkdownDescription)
	}
	synRecv, ok := schemaResp.Schema.Attributes["nf_conntrack_tcp_timeout_syn_recv"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("nf_conntrack_tcp_timeout_syn_recv attribute is not an Int64Attribute")
	}
	if !strings.Contains(synRecv.MarkdownDescription, "Must be between 30 and 60") {
		t.Fatalf("syn recv description must state the range: %q", synRecv.MarkdownDescription)
	}
}

// TestPveNodeFirewallOptionsResource_CreateAndUpdate runs create (PUT then
// read-back GET) and update (PUT with delete query) against a fake API.
func TestPveNodeFirewallOptionsResource_CreateAndUpdate(t *testing.T) {
	r := NewPveNodeFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNodeFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var lastBody []byte
	var lastQuery string
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/nodes/pve1/firewall/options":
			lastBody, _ = io.ReadAll(req.Body)
			lastQuery = req.URL.RawQuery
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"enable":1,"log_level_in":"info","log_level_out":"nolog","log_nf_conntrack":0,"ndp":1,"nf_conntrack_max":262144,"nf_conntrack_tcp_timeout_syn_recv":45,"nosmurfs":1,"tcpflags":0}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	attrTypes := nodeFirewallOptionsAttrTypes()
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
		"node":         tftypes.NewValue(tftypes.String, "pve1"),
		"enable":       tftypes.NewValue(tftypes.Bool, true),
		"log_level_in": tftypes.NewValue(tftypes.String, "debug"),
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
	var created pveNodeFirewallOptionsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "pve1" || !created.Enable.ValueBool() || created.Ndp.ValueBool() != true {
		t.Fatalf("created state = %+v", created)
	}
	if created.NFConntrackMax.ValueInt64() != 262144 || created.Nosmurfs.ValueBool() != true || created.TCPFlags.ValueBool() {
		t.Fatalf("boolish/int decode = %+v", created)
	}
	if !strings.Contains(string(lastBody), `"node":"pve1"`) || !strings.Contains(string(lastBody), `"log_level_in":"debug"`) {
		t.Fatalf("create body = %q", lastBody)
	}

	// Update: log_level_in cleared in the plan must travel in the delete query.
	updateVals := set(map[string]tftypes.Value{
		"node":         tftypes.NewValue(tftypes.String, "pve1"),
		"enable":       tftypes.NewValue(tftypes.Bool, true),
		"log_level_in": tftypes.NewValue(tftypes.String, nil),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "log_level_in") {
		t.Fatalf("update query = %q", lastQuery)
	}
}

// TestPveNodeFirewallOptionsResource_Read404Removes verifies Read drops the
// singleton from state when the node vanished out of band.
func TestPveNodeFirewallOptionsResource_Read404Removes(t *testing.T) {
	r := NewPveNodeFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNodeFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such node"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrTypes := nodeFirewallOptionsAttrTypes()
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	vals["node"] = tftypes.NewValue(tftypes.String, "gone")
	vals["id"] = tftypes.NewValue(tftypes.String, "gone")
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
