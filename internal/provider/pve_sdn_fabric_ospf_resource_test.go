// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnFabricOspfResource_MetadataAndSchema covers the OSPF fabric
// resource's type name and schema shape.
func TestPveSdnFabricOspfResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnFabricOspfResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFabricOspf {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFabricOspf)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"fabric_id", "ip_prefix", "ip6_prefix", "area", "route_filter", "redistribute", "nodes", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["fabric_id"].IsRequired() {
		t.Fatal("fabric_id attribute should be Required")
	}
}

// sdnFabricOspfRedistributeType is the tftypes shape of one redistribute
// entry.
func sdnFabricOspfRedistributeType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"source":    tftypes.String,
		"route_map": tftypes.String,
	}}
}

// sdnFabricOspfIfaceType is the tftypes shape of one OSPF interface entry.
func sdnFabricOspfIfaceType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name":         tftypes.String,
		"ip":           tftypes.String,
		"ip6":          tftypes.String,
		"network_type": tftypes.String,
	}}
}

// sdnFabricOspfNodeType is the tftypes shape of one node member entry.
func sdnFabricOspfNodeType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"node_id":    tftypes.String,
		"ip":         tftypes.String,
		"ip6":        tftypes.String,
		"interfaces": tftypes.List{ElementType: sdnFabricOspfIfaceType()},
	}}
}

// sdnFabricOspfAttrTypes returns the resource's full attribute type map.
func sdnFabricOspfAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"fabric_id":    tftypes.String,
		"ip_prefix":    tftypes.String,
		"ip6_prefix":   tftypes.String,
		"area":         tftypes.String,
		"route_filter": tftypes.String,
		"redistribute": tftypes.List{ElementType: sdnFabricOspfRedistributeType()},
		"nodes":        tftypes.List{ElementType: sdnFabricOspfNodeType()},
		"digest":       tftypes.String,
	}
}

// sdnFabricOspfPlanRaw builds a populated plan value for the create flow.
func sdnFabricOspfPlanRaw() tftypes.Value {
	redistribute := tftypes.NewValue(tftypes.List{ElementType: sdnFabricOspfRedistributeType()}, []tftypes.Value{
		tftypes.NewValue(sdnFabricOspfRedistributeType(), map[string]tftypes.Value{
			"source":    tftypes.NewValue(tftypes.String, "connected"),
			"route_map": tftypes.NewValue(tftypes.String, "rm1"),
		}),
	})
	ifaces := tftypes.NewValue(tftypes.List{ElementType: sdnFabricOspfIfaceType()}, []tftypes.Value{
		tftypes.NewValue(sdnFabricOspfIfaceType(), map[string]tftypes.Value{
			"name":         tftypes.NewValue(tftypes.String, "ens19"),
			"ip":           tftypes.NewValue(tftypes.String, "10.0.0.1/24"),
			"ip6":          tftypes.NewValue(tftypes.String, nil),
			"network_type": tftypes.NewValue(tftypes.String, nil),
		}),
	})
	nodes := tftypes.NewValue(tftypes.List{ElementType: sdnFabricOspfNodeType()}, []tftypes.Value{
		tftypes.NewValue(sdnFabricOspfNodeType(), map[string]tftypes.Value{
			"node_id":    tftypes.NewValue(tftypes.String, "pve1"),
			"ip":         tftypes.NewValue(tftypes.String, "10.0.0.1"),
			"ip6":        tftypes.NewValue(tftypes.String, nil),
			"interfaces": ifaces,
		}),
	})
	vals := map[string]tftypes.Value{
		"fabric_id":    tftypes.NewValue(tftypes.String, "f1"),
		"ip_prefix":    tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"ip6_prefix":   tftypes.NewValue(tftypes.String, nil),
		"area":         tftypes.NewValue(tftypes.String, "0.0.0.0"),
		"route_filter": tftypes.NewValue(tftypes.String, nil),
		"redistribute": redistribute,
		"nodes":        nodes,
		"digest":       tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOspfAttrTypes()}, vals)
}

// TestPveSdnFabricOspfResource_CreateAndDelete runs the full create (fabric
// POST, node POST, read-back) and delete (node DELETE then fabric DELETE)
// paths against a fake API.
func TestPveSdnFabricOspfResource_CreateAndDelete(t *testing.T) {
	exists := false
	var fabricBody, nodeBody []byte
	r := NewPveSdnFabricOspfResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFabricOspfResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/fabrics/fabric":
			fabricBody, _ = io.ReadAll(req.Body)
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/fabrics/node/f1":
			nodeBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such fabric"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"f1","protocol":"ospf","area":"0.0.0.0",`+
				`"ip_prefix":"10.0.0.0/24","redistribute":["source=connected,route-map=rm1"],"digest":"abc123"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/node/f1":
			_, _ = io.WriteString(w, `{"data":[{"fabric_id":"f1","node_id":"pve1","protocol":"ospf","ip":"10.0.0.1",`+
				`"interfaces":["name=ens19,ip=10.0.0.1/24,network_type=broadcast"]}]}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/fabrics/node/f1/pve1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := sdnFabricOspfPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOspfAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnFabricOspfResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	var wireFabric map[string]any
	if err := json.Unmarshal(fabricBody, &wireFabric); err != nil {
		t.Fatalf("fabric body %q is not JSON: %v", fabricBody, err)
	}
	if wireFabric["protocol"] != "ospf" || wireFabric["area"] != "0.0.0.0" {
		t.Fatalf("fabric create body = %+v", wireFabric)
	}
	var wireNode map[string]any
	if err := json.Unmarshal(nodeBody, &wireNode); err != nil {
		t.Fatalf("node body %q is not JSON: %v", nodeBody, err)
	}
	if wireNode["node_id"] != "pve1" || wireNode["protocol"] != "ospf" {
		t.Fatalf("node create body = %+v", wireNode)
	}
	if created.FabricID.ValueString() != "f1" || created.Area.ValueString() != "0.0.0.0" || created.Digest.ValueString() != "abc123" {
		t.Fatalf("created state = %+v", created)
	}
	if len(created.Redistribute) != 1 || created.Redistribute[0].RouteMap.ValueString() != "rm1" {
		t.Fatalf("redistribute = %+v", created.Redistribute)
	}
	if len(created.Nodes) != 1 || len(created.Nodes[0].Interfaces) != 1 ||
		created.Nodes[0].Interfaces[0].NetworkType.ValueString() != "broadcast" {
		t.Fatalf("nodes = %+v", created.Nodes)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
	if exists {
		t.Fatal("fabric was not deleted")
	}
}

// TestPveSdnFabricOspfResource_Read404Removes verifies Read drops the
// resource from state when the fabric vanished out of band.
func TestPveSdnFabricOspfResource_Read404Removes(t *testing.T) {
	r := NewPveSdnFabricOspfResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFabricOspfResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such fabric"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	vals := map[string]tftypes.Value{
		"fabric_id":    tftypes.NewValue(tftypes.String, "gone"),
		"ip_prefix":    tftypes.NewValue(tftypes.String, nil),
		"ip6_prefix":   tftypes.NewValue(tftypes.String, nil),
		"area":         tftypes.NewValue(tftypes.String, nil),
		"route_filter": tftypes.NewValue(tftypes.String, nil),
		"redistribute": tftypes.NewValue(tftypes.List{ElementType: sdnFabricOspfRedistributeType()}, nil),
		"nodes":        tftypes.NewValue(tftypes.List{ElementType: sdnFabricOspfNodeType()}, nil),
		"digest":       tftypes.NewValue(tftypes.String, nil),
	}
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOspfAttrTypes()}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
