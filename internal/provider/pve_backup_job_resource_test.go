// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// backupRawFromSchema builds a full object value whose types come from the
// schema itself, filling every attribute with null unless an override is
// supplied. Works for resource and data source schemas alike.
func backupRawFromSchema[A interface{ GetType() attr.Type }](ctx context.Context, t *testing.T, attrs map[string]A, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	attrTypes := map[string]tftypes.Type{}
	vals := map[string]tftypes.Value{}
	for name, a := range attrs {
		typ := a.GetType().TerraformType(ctx)
		attrTypes[name] = typ
		if v, ok := overrides[name]; ok {
			vals[name] = v
			continue
		}
		vals[name] = tftypes.NewValue(typ, nil)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// newBackupTestClient spins up a fake PVE API served by h and returns a
// client pointed at it. Token auth avoids the /access/ticket exchange.
func newBackupTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
	t.Helper()
	return newHaTestClient(t, h)
}

// TestPveBackupJobResource_MetadataAndSchema covers the backup job
// resource's type name and schema shape.
func TestPveBackupJobResource_MetadataAndSchema(t *testing.T) {
	r := NewPveBackupJobResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveBackupJob {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveBackupJob)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "schedule", "mode", "compress", "storage", "vmid", "exclude_path", "prune_backups", "performance", "fleecing", "next_run"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["next_run"].IsComputed() {
		t.Fatal("next_run attribute should be Computed")
	}
}

// TestPveBackupJobResource_Read404Removes verifies Read drops the resource
// from state when the job vanished out of band.
func TestPveBackupJobResource_Read404Removes(t *testing.T) {
	r := NewPveBackupJobResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveBackupJobResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newBackupTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":"no such job"}`))
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "gone"),
	})
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
