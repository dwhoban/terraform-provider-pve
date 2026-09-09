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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnZoneVlanResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: the zone key forces recreation, the
// vlan-only fields are present, and digest is computed.
func TestPveSdnZoneVlanResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnZoneVlanResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneVlan {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneVlan)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest", "bridge", "bridge_disable_mac_learning"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"tag", "vlan_protocol", "peers", "vxlan_port", "controller", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] != nil {
			t.Fatalf("vlan zone schema must not carry %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["digest"].IsComputed() {
		t.Fatal("digest attribute should be Computed")
	}
}

// TestPveSdnZoneVlanResource_CreateAndDelete runs the create (POST
// /cluster/sdn/zones then read-back GET) and delete (DELETE, including
// already-absent) paths against a fake API.
func TestPveSdnZoneVlanResource_CreateAndDelete(t *testing.T) {
	exists := false
	deletes := 0
	r := NewPveSdnZoneVlanResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnZoneVlanResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/zones":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"zone":"vz1"`, `"type":"vlan"`, `"bridge":"vmbr0"`, `"bridge-disable-mac-learning":true`, `"mtu":1500`, `"nodes":"pve1"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/zones/vz1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"zone 'vz1' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"zone":"vz1","type":"vlan","bridge":"vmbr0",`+
				`"bridge-disable-mac-learning":1,"mtu":"1500","nodes":"pve1","digest":"cafe1234"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/zones/vz1":
			deletes++
			if deletes > 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"zone 'vz1' does not exist"}`)
				return
			}
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := sdnZoneTestObject(t, schemaResp.Schema, map[string]tftypes.Value{
		"zone":                        tftypes.NewValue(tftypes.String, "vz1"),
		"bridge":                      tftypes.NewValue(tftypes.String, "vmbr0"),
		"bridge_disable_mac_learning": tftypes.NewValue(tftypes.Bool, true),
		"mtu":                         tftypes.NewValue(tftypes.Number, 1500),
		"nodes":                       tftypes.NewValue(tftypes.String, "pve1"),
	})
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: sdnZoneTestAttrTypes(t, schemaResp.Schema)}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnZoneVlanResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Zone.ValueString() != "vz1" || created.Bridge.ValueString() != "vmbr0" || !created.BridgeDisableMacLearning.ValueBool() {
		t.Fatalf("created zone = %+v", created)
	}
	if created.MTU.ValueInt64() != 1500 || created.Digest.ValueString() != "cafe1234" || created.Nodes.ValueString() != "pve1" {
		t.Fatalf("created read-back fields = %+v", created)
	}

	// First delete succeeds; the second hits the already-absent 404 and
	// must still succeed.
	for i := range 2 {
		deleteResp := &resource.DeleteResponse{}
		r.Delete(ctx, resource.DeleteRequest{
			State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
		}, deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("Delete %d diagnostics: %s", i, diagnosticsError(deleteResp.Diagnostics))
		}
	}
}
