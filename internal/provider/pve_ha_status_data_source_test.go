// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// newHaTestClient spins up a fake PVE API served by h and returns a client
// pointed at it. Token auth avoids the /access/ticket exchange.
func newHaTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := pveclient.NewClient(pveclient.Credentials{
		Endpoint: srv.URL,
		Token:    "root@pam!test=00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// haDSSchema returns the data source's schema via its Schema method.
func haDSSchema(t *testing.T, d datasource.DataSource, ctx context.Context) dsschema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, resp)
	return resp.Schema
}

// haDSAttrTypes maps the schema's attribute names to their Terraform types.
func haDSAttrTypes(ctx context.Context, s dsschema.Schema) map[string]tftypes.Type {
	attrTypes := make(map[string]tftypes.Type, len(s.Attributes))
	for name, attr := range s.Attributes {
		attrTypes[name] = attr.GetType().TerraformType(ctx)
	}
	return attrTypes
}

// haDSConfig builds a data-source configuration object setting the given
// top-level attributes and leaving every other attribute null.
func haDSConfig(t *testing.T, d datasource.DataSource, ctx context.Context, set map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	s := haDSSchema(t, d, ctx)
	attrTypes := haDSAttrTypes(ctx, s)
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		if v, ok := set[name]; ok {
			vals[name] = v
			continue
		}
		vals[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)}
}

// haDSNullState builds an all-null state container for a data source read.
func haDSNullState(t *testing.T, d datasource.DataSource, ctx context.Context) tfsdk.State {
	t.Helper()
	s := haDSSchema(t, d, ctx)
	attrTypes := haDSAttrTypes(ctx, s)
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)}
}

// TestPveHaStatusDataSource_MetadataAndSchema covers the HA status data
// source's type name and schema shape.
func TestPveHaStatusDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveHaStatusDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaStatus {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaStatus)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "quorate", "quorum_status", "master_node", "nodes", "manager_status", "entries"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveHaStatusDataSource_Read verifies the status decode, the derived
// quorum/master/nodes attributes, and the manager_status document decode.
func TestPveHaStatusDataSource_Read(t *testing.T) {
	d := NewPveHaStatusDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveHaStatusDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/cluster/ha/status/current":
			_, _ = fmt.Fprintf(w, `{"data":[`+
				`{"id":"quorum","type":"quorum","node":"pve1","quorate":1,"status":"OK"},`+
				`{"id":"master","type":"master","node":"pve1","status":"M"},`+
				`{"id":"lrm:pve2","type":"lrm","node":"pve2","status":"idle"},`+
				`{"id":"service:vm:100","type":"service","node":"pve1","sid":"vm:100","state":"started"}]}`)
		case "/cluster/ha/status/manager_status":
			_, _ = fmt.Fprintf(w, `{"data":{"manager_status":{"quorate":1}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	req := datasource.ReadRequest{Config: haDSConfig(t, d, ctx, nil)}
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveHaStatusDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if !got.Quorate.ValueBool() || got.QuorumStatus.ValueString() != "OK" || got.MasterNode.ValueString() != "pve1" {
		t.Fatalf("derived attrs = quorate:%v quorum:%q master:%q", got.Quorate.ValueBool(), got.QuorumStatus.ValueString(), got.MasterNode.ValueString())
	}
	if len(got.Nodes.Elements()) != 2 || len(got.Entries) != 4 {
		t.Fatalf("nodes = %v, entries = %d", got.Nodes.Elements(), len(got.Entries))
	}
	if got.Entries[3].SID.ValueString() != "vm:100" {
		t.Fatalf("service entry sid = %q", got.Entries[3].SID.ValueString())
	}
	if got.ManagerStatus.IsNull() {
		t.Fatal("manager_status should carry the JSON document")
	}
}
