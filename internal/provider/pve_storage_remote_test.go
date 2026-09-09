// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveStorageCephfsResource_MetadataAndSchema covers the resource's
// full type name and the required/sensitive attribute contract.
func TestPveStorageCephfsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageCephfsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageCephfs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageCephfs)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "disable", "nodes", "monhost", "fs_name",
		"subdir", "path", "fuse", "username", "keyring", "digest",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() {
		t.Fatal("storage should be Required")
	}
	if !attrs["keyring"].IsSensitive() {
		t.Fatal("keyring attribute should be Sensitive")
	}
	if !attrs["digest"].IsComputed() {
		t.Fatal("digest attribute should be Computed")
	}
}

// TestPveStorageRbdResource_MetadataAndSchema covers the resource's full
// type name and the required/sensitive attribute contract.
func TestPveStorageRbdResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageRbdResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageRbd {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageRbd)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "disable", "nodes", "monhost", "pool",
		"namespace", "data_pool", "username", "authsupported", "keyring", "krbd", "digest",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() || !attrs["pool"].IsRequired() {
		t.Fatal("storage and pool should be Required")
	}
	if !attrs["keyring"].IsSensitive() {
		t.Fatal("keyring attribute should be Sensitive")
	}
	if !attrs["digest"].IsComputed() {
		t.Fatal("digest attribute should be Computed")
	}
}

// TestPveStoragePbsDataSource_Identity verifies the data source decodes
// the pbs configuration, including the computed identity attributes
// (fingerprint, master_pubkey) and the 0/1 boolean encodings.
func TestPveStoragePbsDataSource_Identity(t *testing.T) {
	d := NewPveStoragePbsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveStoragePbsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/storage/pbs1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"type":"pbs","server":"192.168.1.10","datastore":"store1",`+
			`"username":"backup@pbs","fingerprint":"AA:BB","master-pubkey":"bWFzdGVy","port":"8007",`+
			`"content":"backup","disable":1,"digest":"d9"}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := storageRemoteAttrTypes(map[string]tftypes.Type{
		"type":                   tftypes.String,
		"server":                 tftypes.String,
		"port":                   tftypes.Number,
		"datastore":              tftypes.String,
		"username":               tftypes.String,
		"password":               tftypes.String,
		"fingerprint":            tftypes.String,
		"namespace":              tftypes.String,
		"master_pubkey":          tftypes.String,
		"max_protected_backups":  tftypes.Number,
		"prune_backups":          tftypes.String,
		"skip_cert_verification": tftypes.Bool,
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: storageRemoteTestObject(attrTypes, map[string]tftypes.Value{
			"storage": tftypes.NewValue(tftypes.String, "pbs1"),
		})},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveStoragePbsDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.Type.ValueString() != "pbs" || data.Fingerprint.ValueString() != "AA:BB" || data.MasterPubkey.ValueString() != "bWFzdGVy" {
		t.Fatalf("identity attrs = %s/%s/%s", data.Type.ValueString(), data.Fingerprint.ValueString(), data.MasterPubkey.ValueString())
	}
	if data.Port.ValueInt64() != 8007 || data.Digest.ValueString() != "d9" {
		t.Fatalf("read-back scalars = port=%v digest=%v", data.Port, data.Digest)
	}
	if !data.Disable.ValueBool() {
		t.Fatal("disable should decode true from the 0/1 encoding")
	}
}

// TestPveStorageCephfsDataSource_TypeMismatch verifies the data source
// errors when the upstream storage is not a cephfs storage instead of
// silently returning wrong data.
func TestPveStorageCephfsDataSource_TypeMismatch(t *testing.T) {
	d := NewPveStorageCephfsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveStorageCephfsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"type":"rbd","pool":"rbd"}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := storageRemoteAttrTypes(map[string]tftypes.Type{
		"type":     tftypes.String,
		"monhost":  tftypes.String,
		"fs_name":  tftypes.String,
		"subdir":   tftypes.String,
		"path":     tftypes.String,
		"fuse":     tftypes.Bool,
		"username": tftypes.String,
		"keyring":  tftypes.String,
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: storageRemoteTestObject(attrTypes, map[string]tftypes.Value{
			"storage": tftypes.NewValue(tftypes.String, "notcephfs"),
		})},
	}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected a type mismatch error, got none")
	}
	if !strings.Contains(diagnosticsError(readResp.Diagnostics), `rbd`) {
		t.Fatalf("diagnostics should name the upstream type: %s", diagnosticsError(readResp.Diagnostics))
	}
}
