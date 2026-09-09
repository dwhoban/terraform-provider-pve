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

// TestPveFirewallSecurityGroupDataSource_MetadataAndSchema covers the
// security group data source's type name and schema shape.
func TestPveFirewallSecurityGroupDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveFirewallSecurityGroupDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallSecurityGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallSecurityGroup)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"group", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveFirewallSecurityGroupDataSource_Read verifies the metadata read
// decode from the filtered collection listing.
func TestPveFirewallSecurityGroupDataSource_Read(t *testing.T) {
	d := NewPveFirewallSecurityGroupDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveFirewallSecurityGroupDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/firewall/groups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"group":"db","comment":"DB rules"},{"group":"web","comment":"Web rules"}]}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"group": tftypes.NewValue(tftypes.String, "web"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveFirewallSecurityGroupDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Group.ValueString() != "web" || got.Comment.ValueString() != "Web rules" {
		t.Fatalf("group = %+v", got)
	}
}

// TestPveFirewallSecurityGroupDataSource_ReadNotFoundErrors verifies a
// group missing from the listing surfaces an error diagnostic.
func TestPveFirewallSecurityGroupDataSource_ReadNotFoundErrors(t *testing.T) {
	d := NewPveFirewallSecurityGroupDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveFirewallSecurityGroupDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"group": tftypes.NewValue(tftypes.String, "gone"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for a missing group")
	}
}
