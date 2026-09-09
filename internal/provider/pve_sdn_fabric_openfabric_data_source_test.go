// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnFabricOpenfabricDataSource_MetadataAndSchema covers the
// OpenFabric fabric data source's type name and schema shape.
func TestPveSdnFabricOpenfabricDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnFabricOpenfabricDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFabricOpenfabric {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFabricOpenfabric)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"fabric_id", "node", "csnp_interval", "nodes", "digest", "interfaces", "neighbors", "routes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// sdnFabricOpenfabricDataSourceAttrTypes returns the data source's full
// attribute type map.
func sdnFabricOpenfabricDataSourceAttrTypes() map[string]tftypes.Type {
	out := sdnFabricOpenfabricAttrTypes()
	out["node"] = tftypes.String
	out["interfaces"] = tftypes.List{ElementType: sdnFabricRuntimeIfaceType()}
	out["neighbors"] = tftypes.List{ElementType: sdnFabricRuntimeNeighborType()}
	out["routes"] = tftypes.List{ElementType: sdnFabricRuntimeRouteType()}
	return out
}

// TestPveSdnFabricOpenfabricDataSource_ReadWithRuntime reads the fabric,
// its node members, and the per-node runtime state in one pass.
func TestPveSdnFabricOpenfabricDataSource_ReadWithRuntime(t *testing.T) {
	d := NewPveSdnFabricOpenfabricDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnFabricOpenfabricDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/fabric/of1":
			_, _ = w.Write([]byte(`{"data":{"id":"of1","protocol":"openfabric","csnp_interval":5,"hello_interval":10,"digest":"def789"}}`))
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/node/of1":
			_, _ = w.Write([]byte(`{"data":[{"fabric_id":"of1","node_id":"pve1","protocol":"openfabric","interfaces":["name=ens19,hello_multiplier=3"]}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/of1/interfaces":
			_, _ = w.Write([]byte(`{"data":[{"name":"ens19","state":"up","type":"Broadcast"}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/of1/neighbors":
			_, _ = w.Write([]byte(`{"data":[{"neighbor":"10.0.0.2","status":"Full"}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/of1/routes":
			_, _ = w.Write([]byte(`{"data":[{"route":"10.0.2.0/24","via":[]}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := sdnFabricOpenfabricDataSourceAttrTypes()
	nullStr := tftypes.NewValue(tftypes.String, nil)
	nullList := func(t tftypes.Type) tftypes.Value { return tftypes.NewValue(tftypes.List{ElementType: t}, nil) }
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"fabric_id":      tftypes.NewValue(tftypes.String, "of1"),
		"node":           tftypes.NewValue(tftypes.String, "pve1"),
		"ip_prefix":      nullStr,
		"ip6_prefix":     nullStr,
		"csnp_interval":  tftypes.NewValue(tftypes.Number, nil),
		"hello_interval": tftypes.NewValue(tftypes.Number, nil),
		"route_filter":   nullStr,
		"nodes":          nullList(sdnFabricOpenfabricNodeType()),
		"digest":         nullStr,
		"interfaces":     nullList(sdnFabricRuntimeIfaceType()),
		"neighbors":      nullList(sdnFabricRuntimeNeighborType()),
		"routes":         nullList(sdnFabricRuntimeRouteType()),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveSdnFabricOpenfabricDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.CsnpInterval.ValueFloat64() != 5 || data.Digest.ValueString() != "def789" {
		t.Fatalf("fabric fields = %+v", data)
	}
	if len(data.Nodes) != 1 || data.Nodes[0].Interfaces[0].HelloMultiplier.ValueInt64() != 3 {
		t.Fatalf("nodes = %+v", data.Nodes)
	}
	if len(data.Interfaces) != 1 || data.Interfaces[0].Type.ValueString() != "Broadcast" {
		t.Fatalf("runtime interfaces = %+v", data.Interfaces)
	}
	if len(data.Neighbors) != 1 || data.Neighbors[0].Neighbor.ValueString() != "10.0.0.2" {
		t.Fatalf("runtime neighbors = %+v", data.Neighbors)
	}
	if len(data.Routes) != 1 {
		t.Fatalf("runtime routes = %+v", data.Routes)
	}
}
