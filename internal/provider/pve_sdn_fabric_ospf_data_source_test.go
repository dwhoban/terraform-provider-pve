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

// TestPveSdnFabricOspfDataSource_MetadataAndSchema covers the OSPF fabric
// data source's type name and schema shape.
func TestPveSdnFabricOspfDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnFabricOspfDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFabricOspf {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFabricOspf)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"fabric_id", "node", "area", "redistribute", "nodes", "digest", "interfaces", "neighbors", "routes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// sdnFabricRuntimeIfaceType is the tftypes shape of one runtime interface
// row.
func sdnFabricRuntimeIfaceType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name":  tftypes.String,
		"state": tftypes.String,
		"type":  tftypes.String,
	}}
}

// sdnFabricRuntimeNeighborType is the tftypes shape of one runtime neighbor
// row.
func sdnFabricRuntimeNeighborType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"neighbor": tftypes.String,
		"status":   tftypes.String,
		"uptime":   tftypes.String,
	}}
}

// sdnFabricRuntimeRouteType is the tftypes shape of one runtime route row.
func sdnFabricRuntimeRouteType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"route": tftypes.String,
		"via":   tftypes.List{ElementType: tftypes.String},
	}}
}

// sdnFabricOspfDataSourceAttrTypes returns the data source's full attribute
// type map.
func sdnFabricOspfDataSourceAttrTypes() map[string]tftypes.Type {
	out := sdnFabricOspfAttrTypes()
	out["node"] = tftypes.String
	out["interfaces"] = tftypes.List{ElementType: sdnFabricRuntimeIfaceType()}
	out["neighbors"] = tftypes.List{ElementType: sdnFabricRuntimeNeighborType()}
	out["routes"] = tftypes.List{ElementType: sdnFabricRuntimeRouteType()}
	return out
}

// sdnFabricOspfDataSourceConfigRaw builds a config value with the given
// fabric_id and node (nil = null).
func sdnFabricOspfDataSourceConfigRaw(fabricID any, node any) tftypes.Value {
	attrTypes := sdnFabricOspfDataSourceAttrTypes()
	nullStr := tftypes.NewValue(tftypes.String, nil)
	nullList := func(t tftypes.Type) tftypes.Value { return tftypes.NewValue(tftypes.List{ElementType: t}, nil) }
	vals := map[string]tftypes.Value{
		"fabric_id":    tftypes.NewValue(tftypes.String, fabricID),
		"node":         tftypes.NewValue(tftypes.String, node),
		"ip_prefix":    nullStr,
		"ip6_prefix":   nullStr,
		"area":         nullStr,
		"route_filter": nullStr,
		"redistribute": nullList(sdnFabricOspfRedistributeType()),
		"nodes":        nullList(sdnFabricOspfNodeType()),
		"digest":       nullStr,
		"interfaces":   nullList(sdnFabricRuntimeIfaceType()),
		"neighbors":    nullList(sdnFabricRuntimeNeighborType()),
		"routes":       nullList(sdnFabricRuntimeRouteType()),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveSdnFabricOspfDataSource_ReadWithRuntime reads the fabric, its node
// members, and the per-node runtime state in one pass.
func TestPveSdnFabricOspfDataSource_ReadWithRuntime(t *testing.T) {
	d := NewPveSdnFabricOspfDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnFabricOspfDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			_, _ = w.Write([]byte(`{"data":{"id":"f1","protocol":"ospf","area":"0.0.0.0","digest":"abc123"}}`))
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/node/f1":
			_, _ = w.Write([]byte(`{"data":[{"fabric_id":"f1","node_id":"pve1","protocol":"ospf"}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/f1/interfaces":
			_, _ = w.Write([]byte(`{"data":[{"name":"ens19","state":"up","type":"Point-to-Point"}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/f1/neighbors":
			_, _ = w.Write([]byte(`{"data":[{"neighbor":"10.0.0.2","status":"Full"}]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/sdn/fabrics/f1/routes":
			_, _ = w.Write([]byte(`{"data":[{"route":"10.0.2.0/24","via":["10.0.0.2"]}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := sdnFabricOspfDataSourceConfigRaw("f1", "pve1")
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveSdnFabricOspfDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.Area.ValueString() != "0.0.0.0" || data.Digest.ValueString() != "abc123" {
		t.Fatalf("fabric fields = %+v", data)
	}
	if len(data.Nodes) != 1 || data.Nodes[0].NodeID.ValueString() != "pve1" {
		t.Fatalf("nodes = %+v", data.Nodes)
	}
	if len(data.Interfaces) != 1 || data.Interfaces[0].State.ValueString() != "up" {
		t.Fatalf("runtime interfaces = %+v", data.Interfaces)
	}
	if len(data.Neighbors) != 1 || data.Neighbors[0].Neighbor.ValueString() != "10.0.0.2" {
		t.Fatalf("runtime neighbors = %+v", data.Neighbors)
	}
	if len(data.Routes) != 1 || len(data.Routes[0].Via.Elements()) != 1 {
		t.Fatalf("runtime routes = %+v", data.Routes)
	}
}

// TestPveSdnFabricOspfDataSource_ReadWithoutNode keeps the runtime
// attributes null and never touches the node-scoped endpoints.
func TestPveSdnFabricOspfDataSource_ReadWithoutNode(t *testing.T) {
	d := NewPveSdnFabricOspfDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnFabricOspfDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			_, _ = w.Write([]byte(`{"data":{"id":"f1","protocol":"ospf"}}`))
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/node/f1":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := sdnFabricOspfDataSourceConfigRaw("f1", nil)
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveSdnFabricOspfDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.Interfaces != nil || data.Neighbors != nil || data.Routes != nil {
		t.Fatalf("runtime attributes must stay null without node, got %+v", data)
	}
	if len(data.Nodes) != 0 {
		t.Fatalf("nodes = %+v, want empty", data.Nodes)
	}
}
