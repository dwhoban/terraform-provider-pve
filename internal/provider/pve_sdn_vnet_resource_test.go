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

// TestPveSdnVnetResource_MetadataAndSchema covers the vnet resource's type
// name and schema shape.
func TestPveSdnVnetResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnVnetResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnVnet {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnVnet)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"vnet", "zone", "alias", "tag", "vlanaware", "isolate_ports", "state", "digest"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["vnet"].IsRequired() || !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("vnet and zone attributes should be Required")
	}
	if schemaResp.Schema.Attributes["state"].IsComputed() != true {
		t.Fatal("state attribute should be Computed")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["tag"].GetMarkdownDescription(), "16777215") {
		t.Fatal("tag attribute description should enumerate the range")
	}
}

// TestPveSdnVnetResource_CreateAndDelete runs create (POST with the fixed
// vnet type) and delete against a fake API.
func TestPveSdnVnetResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveSdnVnetResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnVnetResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/vnets":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"type":"vnet"`) || !strings.Contains(string(body), `"vnet":"vnet1"`) || !strings.Contains(string(body), `"zone":"zone1"`) {
				t.Fatalf("create body missing type/vnet/zone: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/vnets/vnet1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such vnet"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"vnet":"vnet1","type":"vnet","zone":"zone1","tag":10,"vlanaware":true,"state":"new","digest":"d1"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/vnets/vnet1":
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
		"vnet":          tftypes.String,
		"zone":          tftypes.String,
		"alias":         tftypes.String,
		"tag":           tftypes.Number,
		"vlanaware":     tftypes.Bool,
		"isolate_ports": tftypes.Bool,
		"state":         tftypes.String,
		"digest":        tftypes.String,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"vnet":          tftypes.NewValue(tftypes.String, "vnet1"),
		"zone":          tftypes.NewValue(tftypes.String, "zone1"),
		"alias":         tftypes.NewValue(tftypes.String, nil),
		"tag":           tftypes.NewValue(tftypes.Number, 10),
		"vlanaware":     tftypes.NewValue(tftypes.Bool, true),
		"isolate_ports": tftypes.NewValue(tftypes.Bool, nil),
		"state":         tftypes.NewValue(tftypes.String, nil),
		"digest":        tftypes.NewValue(tftypes.String, nil),
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
	var created pveSdnVnetResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Zone.ValueString() != "zone1" || created.Tag.ValueInt64() != 10 || !created.VlanAware.ValueBool() || created.Digest.ValueString() != "d1" {
		t.Fatalf("created state = %+v", created)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveSdnVnetDataSource_MetadataAndRead asserts the data source type
// name and the per-node mac_vrf runtime read.
func TestPveSdnVnetDataSource_MetadataAndRead(t *testing.T) {
	d := NewPveSdnVnetDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnVnet {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnVnet)
	}

	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnVnetDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/vnets/vnet1":
			_, _ = io.WriteString(w, `{"data":{"vnet":"vnet1","type":"vnet","zone":"zone1","tag":10,"state":"new","digest":"d1"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/vnets/vnet1/mac-vrf":
			_, _ = io.WriteString(w, `{"data":[{"ip":"10.0.0.5","mac":"BC:24:11:00:00:01","nexthop":"10.0.0.1"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Type{
		"vnet":          tftypes.String,
		"node":          tftypes.String,
		"zone":          tftypes.String,
		"alias":         tftypes.String,
		"tag":           tftypes.Number,
		"vlanaware":     tftypes.Bool,
		"isolate_ports": tftypes.Bool,
		"state":         tftypes.String,
		"digest":        tftypes.String,
		"mac_vrf": tftypes.List{
			ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"ip":      tftypes.String,
				"mac":     tftypes.String,
				"nexthop": tftypes.String,
			}},
		},
	}
	macVrfObjectType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"ip":      tftypes.String,
		"mac":     tftypes.String,
		"nexthop": tftypes.String,
	}}
	config := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"vnet":          tftypes.NewValue(tftypes.String, "vnet1"),
		"node":          tftypes.NewValue(tftypes.String, "pve1"),
		"zone":          tftypes.NewValue(tftypes.String, nil),
		"alias":         tftypes.NewValue(tftypes.String, nil),
		"tag":           tftypes.NewValue(tftypes.Number, nil),
		"vlanaware":     tftypes.NewValue(tftypes.Bool, nil),
		"isolate_ports": tftypes.NewValue(tftypes.Bool, nil),
		"state":         tftypes.NewValue(tftypes.String, nil),
		"digest":        tftypes.NewValue(tftypes.String, nil),
		"mac_vrf":       tftypes.NewValue(tftypes.List{ElementType: macVrfObjectType}, []tftypes.Value{}),
	})

	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var state pveSdnVnetDataSourceModel
	if err := readResp.State.Get(ctx, &state); err != nil {
		t.Fatalf("State.Get after read: %v", err)
	}
	if state.Zone.ValueString() != "zone1" {
		t.Fatalf("state = %+v", state)
	}
	var macVrf []pveSdnVnetMacVrfModel
	if diags := state.MacVrf.ElementsAs(ctx, &macVrf, false); diags.HasError() {
		t.Fatalf("ElementsAs: %+v", diags)
	}
	if len(macVrf) != 1 || macVrf[0].IP.ValueString() != "10.0.0.5" || macVrf[0].MAC.ValueString() != "BC:24:11:00:00:01" || macVrf[0].Nexthop.ValueString() != "10.0.0.1" {
		t.Fatalf("mac_vrf = %+v", macVrf)
	}
}
