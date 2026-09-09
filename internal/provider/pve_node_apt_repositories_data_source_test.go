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

// TestPveNodeAptRepositories_MetadataAndSchema covers the APT repositories
// data source's type name and schema shape.
func TestPveNodeAptRepositories_MetadataAndSchema(t *testing.T) {
	d := NewPveNodeAptRepositoriesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeAptRepositories {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeAptRepositories)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "digest", "repositories", "standard_repositories", "infos", "errors"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
}

// TestPveNodeAptRepositories_ReadFlattensFiles verifies the per-file
// repository entries flatten into rows carrying path and index, and that
// infos/errors/standard rows decode.
func TestPveNodeAptRepositories_ReadFlattensFiles(t *testing.T) {
	d := NewPveNodeAptRepositoriesDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNodeAptRepositoriesDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"digest":"dd01","errors":[{"path":"/etc/apt/sources.list.d/broken.list","error":"nope"}],"files":[{"file-type":"list","path":"/etc/apt/sources.list","repositories":[{"Components":["main"],"Enabled":true,"FileType":"list","Suites":["bookworm"],"Types":["deb"],"URIs":["http://deb.debian.org/debian"]},{"Components":["contrib"],"Enabled":false,"FileType":"list","Suites":["bookworm"],"Types":["deb"],"URIs":["http://deb.debian.org/debian"]}]},{"file-type":"sources","path":"/etc/apt/sources.list.d/ceph.sources","repositories":[{"Components":["no-suites-declared"],"Enabled":true,"FileType":"sources","Options":[{"Key":"Signed-By","Values":["/usr/share/keyrings/ceph.gpg"]}],"Suites":["quincy"],"Types":["deb"],"URIs":["https://download.proxmox.com/debian/ceph-quincy"]}]}],"infos":[{"index":"1","kind":"warning","message":"disabled","path":"/etc/apt/sources.list"}],"standard-repos":[{"handle":"enterprise","name":"Enterprise","status":false},{"handle":"no-subscription","name":"No-Subscription"}]}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := aptRepositoriesDataSourceAttrTypes()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, nil),
		"node":                  tftypes.NewValue(tftypes.String, "pve1"),
		"digest":                tftypes.NewValue(tftypes.String, nil),
		"repositories":          tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptRepoRowAttrTypes()}}, nil),
		"standard_repositories": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptStandardRowAttrTypes()}}, nil),
		"infos":                 tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptInfoRowAttrTypes()}}, nil),
		"errors":                tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptErrorRowAttrTypes()}}, nil),
	})
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveNodeAptRepositoriesDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.ID.ValueString() != "pve1" || data.Digest.ValueString() != "dd01" {
		t.Fatalf("unexpected id/digest: %s %s", data.ID.ValueString(), data.Digest.ValueString())
	}
	if len(data.Repositories) != 3 {
		t.Fatalf("repositories = %d rows, want 3", len(data.Repositories))
	}
	if data.Repositories[0].Index.ValueInt64() != 0 || data.Repositories[0].Path.ValueString() != "/etc/apt/sources.list" || !data.Repositories[0].Enabled.ValueBool() {
		t.Fatalf("row 0 = %+v", data.Repositories[0])
	}
	if data.Repositories[1].Index.ValueInt64() != 1 || data.Repositories[1].Enabled.ValueBool() {
		t.Fatalf("row 1 = %+v", data.Repositories[1])
	}
	if data.Repositories[2].Path.ValueString() != "/etc/apt/sources.list.d/ceph.sources" || len(data.Repositories[2].Options) != 1 || data.Repositories[2].Options[0].Key.ValueString() != "Signed-By" {
		t.Fatalf("row 2 = %+v", data.Repositories[2])
	}
	if len(data.StandardRepositories) != 2 {
		t.Fatalf("standard_repositories = %d rows, want 2", len(data.StandardRepositories))
	}
	if data.StandardRepositories[0].Status.ValueBool() || !data.StandardRepositories[1].Status.IsNull() {
		t.Fatalf("standard statuses unexpected: %+v", data.StandardRepositories)
	}
	if len(data.Infos) != 1 || data.Infos[0].Kind.ValueString() != "warning" {
		t.Fatalf("infos = %+v", data.Infos)
	}
	if len(data.Errors) != 1 || data.Errors[0].Path.ValueString() != "/etc/apt/sources.list.d/broken.list" {
		t.Fatalf("errors = %+v", data.Errors)
	}
}

// aptRepositoriesDataSourceAttrTypes returns the top-level attribute types
// of the APT repositories data source.
func aptRepositoriesDataSourceAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":                    tftypes.String,
		"node":                  tftypes.String,
		"digest":                tftypes.String,
		"repositories":          tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptRepoRowAttrTypes()}},
		"standard_repositories": tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptStandardRowAttrTypes()}},
		"infos":                 tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptInfoRowAttrTypes()}},
		"errors":                tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptErrorRowAttrTypes()}},
	}
}

// aptRepoRowAttrTypes returns the attribute types of one flattened
// repository row.
func aptRepoRowAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"path":       tftypes.String,
		"index":      tftypes.Number,
		"comment":    tftypes.String,
		"types":      tftypes.List{ElementType: tftypes.String},
		"uris":       tftypes.List{ElementType: tftypes.String},
		"suites":     tftypes.List{ElementType: tftypes.String},
		"components": tftypes.List{ElementType: tftypes.String},
		"enabled":    tftypes.Bool,
		"options":    tftypes.List{ElementType: tftypes.Object{AttributeTypes: aptOptionRowAttrTypes()}},
	}
}

// aptOptionRowAttrTypes returns the attribute types of one option row.
func aptOptionRowAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"key":    tftypes.String,
		"values": tftypes.List{ElementType: tftypes.String},
	}
}

// aptStandardRowAttrTypes returns the attribute types of one standard
// repository row.
func aptStandardRowAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"handle": tftypes.String,
		"name":   tftypes.String,
		"status": tftypes.Bool,
	}
}

// aptInfoRowAttrTypes returns the attribute types of one info row.
func aptInfoRowAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"index":    tftypes.String,
		"kind":     tftypes.String,
		"message":  tftypes.String,
		"path":     tftypes.String,
		"property": tftypes.String,
	}
}

// aptErrorRowAttrTypes returns the attribute types of one error row.
func aptErrorRowAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"path":  tftypes.String,
		"error": tftypes.String,
	}
}
