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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPvePermissionsDataSource_SchemaAndMetadata covers the type name and
// attribute set of the effective-permissions data source.
func TestPvePermissionsDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPvePermissionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePvePermissions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePvePermissions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "path", "userid", "entries"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["entries"].IsComputed() {
		t.Fatal("entries attribute should be Computed")
	}
}

// TestPvePermissionsDataSource_Read covers the query encoding, the boolish
// propagate decode, and the path-then-privilege sort of the flattened
// entries.
func TestPvePermissionsDataSource_Read(t *testing.T) {
	d := NewPvePermissionsDataSource()
	impl, ok := d.(*pvePermissionsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	var sawQuery string
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/access/permissions" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		sawQuery = req.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{`+
			`"/vms/100":{"PVEVMUser":0,"Administrator":1},`+
			`"/":{"Administrator":true},`+
			`"/storage/local":{"PVEDatastoreAdmin":false}}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	config := aclTestObjectValue(t, schemaResp.Schema.Type(), map[string]tftypes.Value{
		"userid": aclTestStr("root@pam!ci"),
	})
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(sawQuery, "userid=root%40pam%21ci") {
		t.Fatalf("query missing encoded userid: %s", sawQuery)
	}
	var data pvePermissionsDataSourceModel
	if err := resp.State.Get(ctx, &data); err != nil {
		t.Fatalf("get state: %v", err)
	}
	if data.Userid.ValueString() != "root@pam!ci" {
		t.Fatalf("userid = %q", data.Userid.ValueString())
	}
	// Flattened rows must be sorted by path then privilege.
	type row struct {
		path      string
		privilege string
		propagate bool
	}
	want := []row{
		{"/", "Administrator", true},
		{"/storage/local", "PVEDatastoreAdmin", false},
		{"/vms/100", "Administrator", true},
		{"/vms/100", "PVEVMUser", false},
	}
	if len(data.Entries) != len(want) {
		t.Fatalf("len(entries) = %d, want %d (%+v)", len(data.Entries), len(want), data.Entries)
	}
	for i, w := range want {
		got := data.Entries[i]
		if got.Path.ValueString() != w.path || got.Privilege.ValueString() != w.privilege || got.Propagate.ValueBool() != w.propagate {
			t.Fatalf("entries[%d] = (%s, %s, %v), want (%s, %s, %v)",
				i, got.Path.ValueString(), got.Privilege.ValueString(), got.Propagate.ValueBool(), w.path, w.privilege, w.propagate)
		}
	}
}
