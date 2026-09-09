// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnZoneEvpnResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: evpn-only fields carry the pin's
// validators, and the zone key forces recreation.
func TestPveSdnZoneEvpnResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnZoneEvpnResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneEvpn {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneEvpn)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest",
		"controller", "secondary_controllers", "advertise_subnets", "disable_arp_nd_suppression",
		"exitnodes", "exitnodes_local_routing", "exitnodes_primary", "mac", "rt_import", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"bridge", "tag", "vlan_protocol", "peers", "vxlan_port", "fabric"} {
		if schemaResp.Schema.Attributes[key] != nil {
			t.Fatalf("evpn zone schema must not carry %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	vniAttr, ok := schemaResp.Schema.Attributes["vrf_vxlan"].(schema.Int64Attribute)
	if !ok || len(vniAttr.Validators) == 0 {
		t.Fatal("vrf_vxlan attribute should carry a validator")
	}
}

// TestPveSdnZoneEvpnResource_UpdateClearsRemovedFields verifies the PUT
// carries the updatable scalars and translates attributes cleared in the
// plan into the PVE `delete` query parameter, while the immutable zone
// type never travels on update.
func TestPveSdnZoneEvpnResource_UpdateClearsRemovedFields(t *testing.T) {
	var sawQuery url.Values
	var sawBody string
	r := NewPveSdnZoneEvpnResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnZoneEvpnResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodPut:
			if req.URL.Path != "/cluster/sdn/zones/evpn1" {
				t.Fatalf("unexpected PUT path: %s", req.URL.Path)
			}
			sawQuery = req.URL.Query()
			raw, _ := io.ReadAll(req.Body)
			sawBody = string(raw)
			_, _ = io.WriteString(w, `{"data":null}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":{"zone":"evpn1","type":"evpn","controller":"ctrl2","advertise-subnets":1,"vrf-vxlan":100,"digest":"dd44"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	stateRaw := sdnZoneTestObject(t, schemaResp.Schema, map[string]tftypes.Value{
		"zone":              tftypes.NewValue(tftypes.String, "evpn1"),
		"controller":        tftypes.NewValue(tftypes.String, "ctrl1"),
		"rt_import":         tftypes.NewValue(tftypes.String, "65000:100"),
		"mtu":               tftypes.NewValue(tftypes.Number, 1450),
		"advertise_subnets": tftypes.NewValue(tftypes.Bool, false),
	})
	planRaw := sdnZoneTestObject(t, schemaResp.Schema, map[string]tftypes.Value{
		"zone":              tftypes.NewValue(tftypes.String, "evpn1"),
		"controller":        tftypes.NewValue(tftypes.String, "ctrl2"),
		"advertise_subnets": tftypes.NewValue(tftypes.Bool, true),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if got := sawQuery.Get("delete"); got != "mtu,rt-import" {
		t.Fatalf("delete param = %q, want mtu,rt-import", got)
	}
	for _, want := range []string{`"zone":"evpn1"`, `"controller":"ctrl2"`, `"advertise-subnets":true`} {
		if !strings.Contains(sawBody, want) {
			t.Fatalf("update body missing %s: %q", want, sawBody)
		}
	}
	for _, banned := range []string{`"type"`, `"rt-import"`} {
		if strings.Contains(sawBody, banned) {
			t.Fatalf("update body must not carry %s: %q", banned, sawBody)
		}
	}
	var updated pveSdnZoneEvpnResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if updated.Controller.ValueString() != "ctrl2" || !updated.AdvertiseSubnets.ValueBool() || updated.VrfVxlan.ValueInt64() != 100 {
		t.Fatalf("updated read-back fields = %+v", updated)
	}
	if !updated.RtImport.IsNull() || !updated.MTU.IsNull() {
		t.Fatalf("cleared fields must read back null: rt_import=%v mtu=%v", updated.RtImport, updated.MTU)
	}
}
