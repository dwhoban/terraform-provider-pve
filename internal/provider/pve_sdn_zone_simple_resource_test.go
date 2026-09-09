// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnZoneSimpleResource_MetadataAndSchema covers the resource's
// full type name and its schema contract: the zone key forces recreation
// and digest is computed.
func TestPveSdnZoneSimpleResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnZoneSimpleResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneSimple {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneSimple)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"bridge", "tag", "vlan_protocol", "peers", "vxlan_port", "fabric", "controller", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] != nil {
			t.Fatalf("simple zone schema must not carry %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["digest"].IsComputed() {
		t.Fatal("digest attribute should be Computed")
	}
}

// TestPveSdnZoneSimpleResource_Read404Removes verifies Read drops the
// resource from state when the zone vanished out of band.
func TestPveSdnZoneSimpleResource_Read404Removes(t *testing.T) {
	r := NewPveSdnZoneSimpleResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnZoneSimpleResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"zone 'gone' does not exist"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: sdnZoneTestObject(t, schemaResp.Schema, map[string]tftypes.Value{
		"zone": tftypes.NewValue(tftypes.String, "gone"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
