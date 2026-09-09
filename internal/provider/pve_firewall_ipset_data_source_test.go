// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveFirewallIpsetDataSource_MetadataAndSchema covers the ipset data
// source's type name and schema shape.
func TestPveFirewallIpsetDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveFirewallIpsetDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallIpset {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallIpset)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "comment", "cidrs"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveFirewallIpsetDataSource_Read verifies the ipset read decode,
// including member collection from the per-set content listing.
func TestPveFirewallIpsetDataSource_Read(t *testing.T) {
	d := NewPveFirewallIpsetDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveFirewallIpsetDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/ipset/mgmt":
			_, _ = w.Write([]byte(`{"data":[{"cidr":"10.0.0.1"},{"cidr":"192.168.1.0/24","nomatch":1}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/ipset":
			_, _ = w.Write([]byte(`{"data":[{"name":"mgmt","comment":"Admin hosts"}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "mgmt"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveFirewallIpsetDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Name.ValueString() != "mgmt" || got.Comment.ValueString() != "Admin hosts" || len(got.Cidrs.Elements()) != 2 {
		t.Fatalf("ipset = %+v", got)
	}
}
