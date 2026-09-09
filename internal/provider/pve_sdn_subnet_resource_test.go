// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnSubnetResource_MetadataAndSchema covers the subnet resource's
// type name and schema shape.
func TestPveSdnSubnetResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnSubnetResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnSubnet {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnSubnet)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"vnet", "subnet", "gateway", "snat", "dhcp_dns_server", "dhcp_range", "dnszoneprefix"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["vnet"].IsRequired() || !schemaResp.Schema.Attributes["subnet"].IsRequired() {
		t.Fatal("vnet and subnet attributes should be Required")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["subnet"].GetMarkdownDescription(), "10.0.0.0-24") {
		t.Fatal("subnet attribute description should document the hyphenated CIDR form")
	}
}

// TestPveSdnSubnetResource_CreateImportDelete runs create, import-ID
// parsing, and delete against a fake API.
func TestPveSdnSubnetResource_CreateImportDelete(t *testing.T) {
	exists := false
	r := NewPveSdnSubnetResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnSubnetResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/vnets/vnet1/subnets":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"type":"subnet"`) || !strings.Contains(string(body), `"subnet":"10.0.0.0-24"`) {
				t.Fatalf("create body missing type/subnet: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/vnets/vnet1/subnets/10.0.0.0-24":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such subnet"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"subnet":"10.0.0.0-24","type":"subnet","gateway":"10.0.0.1","snat":true,"dhcp-range":["10.0.0.100-10.0.0.200"]}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/vnets/vnet1/subnets/10.0.0.0-24":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Type{
		"vnet":            tftypes.String,
		"subnet":          tftypes.String,
		"gateway":         tftypes.String,
		"snat":            tftypes.Bool,
		"dhcp_dns_server": tftypes.String,
		"dhcp_range":      tftypes.List{ElementType: tftypes.String},
		"dnszoneprefix":   tftypes.String,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"vnet":            tftypes.NewValue(tftypes.String, "vnet1"),
		"subnet":          tftypes.NewValue(tftypes.String, "10.0.0.0-24"),
		"gateway":         tftypes.NewValue(tftypes.String, "10.0.0.1"),
		"snat":            tftypes.NewValue(tftypes.Bool, true),
		"dhcp_dns_server": tftypes.NewValue(tftypes.String, nil),
		"dhcp_range":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "10.0.0.100-10.0.0.200")}),
		"dnszoneprefix":   tftypes.NewValue(tftypes.String, nil),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnSubnetResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Gateway.ValueString() != "10.0.0.1" || !created.Snat.ValueBool() {
		t.Fatalf("created state = %+v", created)
	}
	var ranges []string
	if diags := created.DhcpRange.ElementsAs(ctx, &ranges, false); diags.HasError() {
		t.Fatalf("ElementsAs: %+v", diags)
	}
	if len(ranges) != 1 || ranges[0] != "10.0.0.100-10.0.0.200" {
		t.Fatalf("dhcp_range = %+v", ranges)
	}

	// Import parses the `<vnet>:<subnet>` ID.
	importResp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	impl.ImportState(ctx, resource.ImportStateRequest{ID: "vnet1:10.0.0.0-24"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("Import diagnostics: %s", diagnosticsError(importResp.Diagnostics))
	}
	var imported pveSdnSubnetResourceModel
	if err := importResp.State.Get(ctx, &imported); err != nil {
		t.Fatalf("State.Get after import: %v", err)
	}
	if imported.Vnet.ValueString() != "vnet1" || imported.Subnet.ValueString() != "10.0.0.0-24" {
		t.Fatalf("imported state = %+v", imported)
	}

	// Delete removes the subnet upstream.
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveSdnSubnetDataSource_Metadata asserts the data source type name.
func TestPveSdnSubnetDataSource_Metadata(t *testing.T) {
	d := NewPveSdnSubnetDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnSubnet {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnSubnet)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["subnet"].IsRequired() {
		t.Fatal("subnet attribute should be Required")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["dhcp_range"].GetMarkdownDescription(), "start-end") {
		t.Fatal("dhcp_range description should document the range format")
	}
}
