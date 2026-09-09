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

// TestPveSdnFabricOpenfabricResource_MetadataAndSchema covers the
// OpenFabric fabric resource's type name and schema shape.
func TestPveSdnFabricOpenfabricResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnFabricOpenfabricResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFabricOpenfabric {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFabricOpenfabric)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"fabric_id", "ip_prefix", "ip6_prefix", "csnp_interval", "hello_interval", "route_filter", "nodes", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["fabric_id"].IsRequired() {
		t.Fatal("fabric_id attribute should be Required")
	}
}

// sdnFabricOpenfabricIfaceType is the tftypes shape of one OpenFabric
// interface entry.
func sdnFabricOpenfabricIfaceType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name":             tftypes.String,
		"ip":               tftypes.String,
		"ip6":              tftypes.String,
		"hello_multiplier": tftypes.Number,
	}}
}

// sdnFabricOpenfabricNodeType is the tftypes shape of one node member entry.
func sdnFabricOpenfabricNodeType() tftypes.Type {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"node_id":    tftypes.String,
		"ip":         tftypes.String,
		"ip6":        tftypes.String,
		"interfaces": tftypes.List{ElementType: sdnFabricOpenfabricIfaceType()},
	}}
}

// sdnFabricOpenfabricAttrTypes returns the resource's full attribute type
// map.
func sdnFabricOpenfabricAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"fabric_id":      tftypes.String,
		"ip_prefix":      tftypes.String,
		"ip6_prefix":     tftypes.String,
		"csnp_interval":  tftypes.Number,
		"hello_interval": tftypes.Number,
		"route_filter":   tftypes.String,
		"nodes":          tftypes.List{ElementType: sdnFabricOpenfabricNodeType()},
		"digest":         tftypes.String,
	}
}

// sdnFabricOpenfabricPlanRaw builds a populated plan value.
func sdnFabricOpenfabricPlanRaw() tftypes.Value {
	ifaces := tftypes.NewValue(tftypes.List{ElementType: sdnFabricOpenfabricIfaceType()}, []tftypes.Value{
		tftypes.NewValue(sdnFabricOpenfabricIfaceType(), map[string]tftypes.Value{
			"name":             tftypes.NewValue(tftypes.String, "ens19"),
			"ip":               tftypes.NewValue(tftypes.String, nil),
			"ip6":              tftypes.NewValue(tftypes.String, nil),
			"hello_multiplier": tftypes.NewValue(tftypes.Number, nil),
		}),
	})
	nodes := tftypes.NewValue(tftypes.List{ElementType: sdnFabricOpenfabricNodeType()}, []tftypes.Value{
		tftypes.NewValue(sdnFabricOpenfabricNodeType(), map[string]tftypes.Value{
			"node_id":    tftypes.NewValue(tftypes.String, "pve1"),
			"ip":         tftypes.NewValue(tftypes.String, nil),
			"ip6":        tftypes.NewValue(tftypes.String, nil),
			"interfaces": ifaces,
		}),
	})
	vals := map[string]tftypes.Value{
		"fabric_id":      tftypes.NewValue(tftypes.String, "of1"),
		"ip_prefix":      tftypes.NewValue(tftypes.String, "10.0.0.0/24"),
		"ip6_prefix":     tftypes.NewValue(tftypes.String, nil),
		"csnp_interval":  tftypes.NewValue(tftypes.Number, 5),
		"hello_interval": tftypes.NewValue(tftypes.Number, 10),
		"route_filter":   tftypes.NewValue(tftypes.String, nil),
		"nodes":          nodes,
		"digest":         tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOpenfabricAttrTypes()}, vals)
}

// TestPveSdnFabricOpenfabricResource_CreateAndDelete runs the full create
// (fabric POST, node POST, read-back) and delete (node DELETE then fabric
// DELETE) paths against a fake API.
func TestPveSdnFabricOpenfabricResource_CreateAndDelete(t *testing.T) {
	exists := false
	var fabricBody, nodeBody []byte
	r := NewPveSdnFabricOpenfabricResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFabricOpenfabricResource)
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
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/fabrics/node/of1":
			nodeBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/fabric/of1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such fabric"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"of1","protocol":"openfabric","ip_prefix":"10.0.0.0/24",`+
				`"csnp_interval":5,"hello_interval":10,"digest":"def789"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/fabrics/node/of1":
			_, _ = io.WriteString(w, `{"data":[{"fabric_id":"of1","node_id":"pve1","protocol":"openfabric",`+
				`"interfaces":[{"name":"ens19","hello_multiplier":3}]}]}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/fabrics/node/of1/pve1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/fabrics/fabric/of1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := sdnFabricOpenfabricPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOpenfabricAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnFabricOpenfabricResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	var wireFabric map[string]any
	if err := json.Unmarshal(fabricBody, &wireFabric); err != nil {
		t.Fatalf("fabric body %q is not JSON: %v", fabricBody, err)
	}
	if wireFabric["protocol"] != "openfabric" || wireFabric["csnp_interval"] != float64(5) {
		t.Fatalf("fabric create body = %+v", wireFabric)
	}
	var wireNode map[string]any
	if err := json.Unmarshal(nodeBody, &wireNode); err != nil {
		t.Fatalf("node body %q is not JSON: %v", nodeBody, err)
	}
	if wireNode["node_id"] != "pve1" || wireNode["protocol"] != "openfabric" {
		t.Fatalf("node create body = %+v", wireNode)
	}
	if created.FabricID.ValueString() != "of1" || created.Digest.ValueString() != "def789" ||
		created.CsnpInterval.ValueFloat64() != 5 || created.HelloInterval.ValueFloat64() != 10 {
		t.Fatalf("created state = %+v", created)
	}
	if len(created.Nodes) != 1 || len(created.Nodes[0].Interfaces) != 1 ||
		created.Nodes[0].Interfaces[0].HelloMultiplier.ValueInt64() != 3 {
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

// TestPveSdnFabricOpenfabricResource_Read404Removes verifies Read drops the
// resource from state when the fabric vanished out of band.
func TestPveSdnFabricOpenfabricResource_Read404Removes(t *testing.T) {
	r := NewPveSdnFabricOpenfabricResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFabricOpenfabricResource)
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
		"fabric_id":      tftypes.NewValue(tftypes.String, "gone"),
		"ip_prefix":      tftypes.NewValue(tftypes.String, nil),
		"ip6_prefix":     tftypes.NewValue(tftypes.String, nil),
		"csnp_interval":  tftypes.NewValue(tftypes.Number, nil),
		"hello_interval": tftypes.NewValue(tftypes.Number, nil),
		"route_filter":   tftypes.NewValue(tftypes.String, nil),
		"nodes":          tftypes.NewValue(tftypes.List{ElementType: sdnFabricOpenfabricNodeType()}, nil),
		"digest":         tftypes.NewValue(tftypes.String, nil),
	}
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: sdnFabricOpenfabricAttrTypes()}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
