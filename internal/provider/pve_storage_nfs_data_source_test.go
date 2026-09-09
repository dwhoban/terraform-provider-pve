// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveStorageNfsDataSource_MetadataAndSchema covers the data source's
// full type name and its read shape: the storage lookup key is required
// and every other attribute is computed.
func TestPveStorageNfsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveStorageNfsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageNfs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageNfs)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"server", "export", "options",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() {
		t.Fatal("storage should be Required")
	}
	for _, key := range []string{"server", "export", "content", "digest"} {
		if !attrs[key].IsComputed() {
			t.Fatalf("%s should be Computed", key)
		}
	}
}

// TestPveStorageNfsDataSource_Read verifies the read path decodes the
// storage config, splitting the comma-separated content list into a set.
func TestPveStorageNfsDataSource_Read(t *testing.T) {
	d := NewPveStorageNfsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveStorageNfsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/storage/media" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"storage":"media","type":"nfs","server":"192.168.1.10",`+
			`"export":"/srv/export","content":"images,iso","nodes":"pve1","options":"vers=4.2",`+
			`"shared":true,"max-protected-backups":"5","digest":"cafe1234"}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configRaw := storageNfsTestObject(map[string]tftypes.Value{
		"storage": tftypes.NewValue(tftypes.String, "media"),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: storageNfsAttrTypes()}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configRaw},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var got pveStorageNfsDataSourceModel
	if err := readResp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get after read: %v", err)
	}
	if got.Server.ValueString() != "192.168.1.10" || got.Export.ValueString() != "/srv/export" {
		t.Fatalf("read identity = %s/%s", got.Server.ValueString(), got.Export.ValueString())
	}
	if len(got.Content.Elements()) != 2 {
		t.Fatalf("content = %v, want 2 elements", got.Content.Elements())
	}
	if got.Nodes.ValueString() != "pve1" || got.Options.ValueString() != "vers=4.2" {
		t.Fatalf("read list fields = %s/%s", got.Nodes.ValueString(), got.Options.ValueString())
	}
	if !got.Shared.ValueBool() || got.MaxProtectedBackups.ValueInt64() != 5 {
		t.Fatalf("read scalars = shared=%v max=%v", got.Shared.ValueBool(), got.MaxProtectedBackups.ValueInt64())
	}
	if got.Digest.ValueString() != "cafe1234" {
		t.Fatalf("digest = %s", got.Digest.ValueString())
	}
}
