// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// aclTestObjectValue builds a tftypes object value for the given schema
// type, filling named attributes from values and leaving the rest null.
func aclTestObjectValue(t *testing.T, typ attr.Type, values map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	objType, ok := typ.TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type %s is not an object", typ)
	}
	attrs := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, present := values[name]; present {
			attrs[name] = v
			continue
		}
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(objType, attrs)
}

// aclTestResourceSchema returns the pve_acl resource with its metadata
// asserted and schema built.
func aclTestResourceSchema(t *testing.T) (resource.Resource, resource.SchemaResponse) {
	t.Helper()
	r := NewPveAclResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcl {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcl)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	return r, *schemaResp
}

// aclTestStr is a shorthand for a non-null tftypes string value.
func aclTestStr(s string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, s)
}

// TestPveAclResource_MetadataAndSchema covers the type name, attribute set,
// and requiredness of the identity keys.
func TestPveAclResource_MetadataAndSchema(t *testing.T) {
	r, schemaResp := aclTestResourceSchema(t)
	for _, key := range []string{"path", "role", "type", "user_id", "group_id", "token_id", "propagate"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"path", "role", "type"} {
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
	if !schemaResp.Schema.Attributes["propagate"].IsComputed() {
		t.Fatal("propagate attribute should be Computed")
	}
	if _, ok := r.(resource.ResourceWithValidateConfig); !ok {
		t.Fatal("resource should implement ResourceWithValidateConfig")
	}
}

// TestPveAclResource_ValidateConfig enforces the exactly-one-identity rule
// and the type/identity match.
func TestPveAclResource_ValidateConfig(t *testing.T) {
	r, schemaResp := aclTestResourceSchema(t)
	vc, ok := r.(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("resource should implement ResourceWithValidateConfig")
	}
	for _, tc := range []struct {
		name      string
		userID    string
		groupID   string
		tokenID   string
		aclType   string
		wantError bool
	}{
		{name: "user matches", userID: "ops@pam", aclType: "user"},
		{name: "group matches", groupID: "ops", aclType: "group"},
		{name: "token matches", tokenID: "ops@pam!ci", aclType: "token"},
		{name: "no identity", aclType: "user", wantError: true},
		{name: "mismatched identity", groupID: "ops", aclType: "user", wantError: true},
		{name: "two identities", userID: "ops@pam", groupID: "ops", aclType: "user", wantError: true},
	} {
		values := map[string]tftypes.Value{
			"path": aclTestStr("/vms/100"),
			"role": aclTestStr("PVEVMUser"),
			"type": aclTestStr(tc.aclType),
		}
		for name, v := range map[string]string{"user_id": tc.userID, "group_id": tc.groupID, "token_id": tc.tokenID} {
			if v != "" {
				values[name] = aclTestStr(v)
			}
		}
		config := aclTestObjectValue(t, schemaResp.Schema.Type(), values)
		req := resource.ValidateConfigRequest{
			Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config},
		}
		resp := &resource.ValidateConfigResponse{}
		vc.ValidateConfig(context.Background(), req, resp)
		if tc.wantError && !resp.Diagnostics.HasError() {
			t.Fatalf("%s: expected an error", tc.name)
		}
		if !tc.wantError && resp.Diagnostics.HasError() {
			t.Fatalf("%s: unexpected error: %s", tc.name, diagnosticsError(resp.Diagnostics))
		}
	}
}

// TestPveAclResource_CRUD drives Create, Read (present and absent) and
// Delete against a fake /access/acl, asserting the wire shapes: PUT on
// create, PUT with delete=true on delete, and removal from state when the
// entry disappears.
func TestPveAclResource_CRUD(t *testing.T) {
	r, schemaResp := aclTestResourceSchema(t)
	impl, ok := r.(*pveAclResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var puts []map[string]any
	aclJSON := `[{"path":"/vms/100","roleid":"PVEVMUser","type":"user","ugid":"ops@pam","propagate":1}]`
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/access/acl":
			raw, _ := io.ReadAll(req.Body)
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			puts = append(puts, body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/access/acl":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":`+aclJSON+`}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	objType := schemaResp.Schema.Type().TerraformType(context.Background())
	planValue := aclTestObjectValue(t, schemaResp.Schema.Type(), map[string]tftypes.Value{
		"path":      aclTestStr("/vms/100"),
		"role":      aclTestStr("PVEVMUser"),
		"type":      aclTestStr("user"),
		"user_id":   aclTestStr("ops@pam"),
		"propagate": tftypes.NewValue(tftypes.Bool, true),
	})

	// Create.
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.Create(context.Background(), resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planValue},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planValue},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %s", diagnosticsError(createResp.Diagnostics))
	}
	if len(puts) != 1 {
		t.Fatalf("expected one PUT after create, got %d", len(puts))
	}
	if puts[0]["path"] != "/vms/100" || puts[0]["roles"] != "PVEVMUser" || puts[0]["users"] != "ops@pam" || puts[0]["propagate"] != true {
		t.Fatalf("create PUT body = %v", puts[0])
	}
	var created pveAclResourceModel
	if err := createResp.State.Get(context.Background(), &created); err != nil {
		t.Fatalf("get created state: %v", err)
	}
	if !created.Propagate.ValueBool() {
		t.Fatalf("created propagate = %v, want true from server value 1", created.Propagate)
	}

	// Read (entry present, propagate flips to false server-side).
	aclJSON = `[{"path":"/vms/100","roleid":"PVEVMUser","type":"user","ugid":"ops@pam","propagate":0}]`
	readResp := &resource.ReadResponse{State: createResp.State}
	r.Read(context.Background(), resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %s", diagnosticsError(readResp.Diagnostics))
	}
	var read pveAclResourceModel
	if err := readResp.State.Get(context.Background(), &read); err != nil {
		t.Fatalf("get read state: %v", err)
	}
	if read.Propagate.ValueBool() {
		t.Fatalf("read propagate = %v, want false from server value 0", read.Propagate)
	}

	// Read again with the entry gone: the resource must leave state.
	aclJSON = `[]`
	absentResp := &resource.ReadResponse{State: readResp.State}
	r.Read(context.Background(), resource.ReadRequest{State: readResp.State}, absentResp)
	if absentResp.Diagnostics.HasError() {
		t.Fatalf("absent read: %s", diagnosticsError(absentResp.Diagnostics))
	}
	if !absentResp.State.Raw.IsNull() {
		t.Fatalf("state should be removed for an absent entry, got %s", absentResp.State.Raw)
	}

	// Delete.
	delResp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("delete: %s", diagnosticsError(delResp.Diagnostics))
	}
	if len(puts) != 2 {
		t.Fatalf("expected a second PUT for delete, got %d", len(puts))
	}
	if puts[1]["delete"] != true {
		t.Fatalf("delete PUT body = %v, want delete=true", puts[1])
	}
	if puts[1]["path"] != "/vms/100" || puts[1]["users"] != "ops@pam" || puts[1]["roles"] != "PVEVMUser" {
		t.Fatalf("delete PUT body = %v", puts[1])
	}
}

// TestPveAclResource_UpdateReputsEntry verifies Update is an idempotent PUT
// (only `propagate` can change; identity keys force replacement).
func TestPveAclResource_UpdateReputsEntry(t *testing.T) {
	r, schemaResp := aclTestResourceSchema(t)
	impl, ok := r.(*pveAclResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var putBody map[string]any
	aclJSON := `[{"path":"/vms/100","roleid":"PVEVMUser","type":"user","ugid":"ops@pam","propagate":false}]`
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/access/acl":
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/access/acl":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":`+aclJSON+`}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	objType := schemaResp.Schema.Type().TerraformType(context.Background())
	planValue := aclTestObjectValue(t, schemaResp.Schema.Type(), map[string]tftypes.Value{
		"path":      aclTestStr("/vms/100"),
		"role":      aclTestStr("PVEVMUser"),
		"type":      aclTestStr("user"),
		"user_id":   aclTestStr("ops@pam"),
		"propagate": tftypes.NewValue(tftypes.Bool, true),
	})
	stateValue := aclTestObjectValue(t, schemaResp.Schema.Type(), map[string]tftypes.Value{
		"path":      aclTestStr("/vms/100"),
		"role":      aclTestStr("PVEVMUser"),
		"type":      aclTestStr("user"),
		"user_id":   aclTestStr("ops@pam"),
		"propagate": tftypes.NewValue(tftypes.Bool, false),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.Update(context.Background(), resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planValue},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planValue},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateValue},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if putBody["propagate"] != true || putBody["delete"] != nil {
		t.Fatalf("update PUT body = %v, want propagate=true without delete", putBody)
	}
}

// TestPveAclResource_ImportState covers the `<path>|<role>|<type>|<id>`
// import contract.
func TestPveAclResource_ImportState(t *testing.T) {
	r, schemaResp := aclTestResourceSchema(t)
	ctx := context.Background()
	objType := schemaResp.Schema.Type().TerraformType(ctx)
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	importer, ok := r.(resource.ResourceWithImportState)
	if !ok {
		t.Fatal("resource should implement ResourceWithImportState")
	}
	importer.ImportState(ctx, resource.ImportStateRequest{ID: "/vms/100|PVEVMUser|user|ops@pam"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %s", diagnosticsError(resp.Diagnostics))
	}
	var m pveAclResourceModel
	if err := resp.State.Get(ctx, &m); err != nil {
		t.Fatalf("get imported state: %v", err)
	}
	if m.Path.ValueString() != "/vms/100" || m.Role.ValueString() != "PVEVMUser" || m.Type.ValueString() != "user" || m.UserID.ValueString() != "ops@pam" {
		t.Fatalf("imported = path %q role %q type %q user %q", m.Path.ValueString(), m.Role.ValueString(), m.Type.ValueString(), m.UserID.ValueString())
	}
	if !m.GroupID.IsNull() || !m.TokenID.IsNull() {
		t.Fatalf("unrelated identity attrs should stay null, got group %q token %q", m.GroupID.ValueString(), m.TokenID.ValueString())
	}
}

// TestAclParseImportID covers the parser's acceptance and rejection cases.
func TestAclParseImportID(t *testing.T) {
	for _, tc := range []struct {
		id        string
		path      string
		role      string
		typ       string
		ugid      string
		wantError bool
	}{
		{id: "/vms/100|PVEVMUser|user|ops@pam", path: "/vms/100", role: "PVEVMUser", typ: "user", ugid: "ops@pam"},
		{id: "/|Administrator|group|ops", path: "/", role: "Administrator", typ: "group", ugid: "ops"},
		{id: "/vms/100|Operator|token|ops@pam!ci", path: "/vms/100", role: "Operator", typ: "token", ugid: "ops@pam!ci"},
		{id: "/vms/100|PVEVMUser|user", wantError: true},
		{id: "|PVEVMUser|user|ops@pam", wantError: true},
		{id: "/vms/100|PVEVMUser|role|ops@pam", wantError: true},
	} {
		path, role, typ, ugid, err := aclParseImportID(tc.id)
		if tc.wantError {
			if err == nil {
				t.Fatalf("id %q: expected error", tc.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("id %q: %v", tc.id, err)
		}
		if path != tc.path || role != tc.role || typ != tc.typ || ugid != tc.ugid {
			t.Fatalf("id %q: got (%s,%s,%s,%s), want (%s,%s,%s,%s)", tc.id, path, role, typ, ugid, tc.path, tc.role, tc.typ, tc.ugid)
		}
	}
}

// TestPveAclDataSource_SchemaAndMetadata covers the ACL list data source.
func TestPveAclDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveAclDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcl {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcl)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "entries"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveAclDataSource_Read decodes the ACL list into entries, including
// the boolish propagate encodings.
func TestPveAclDataSource_Read(t *testing.T) {
	d := NewPveAclDataSource()
	impl, ok := d.(*pveAclDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/access/acl" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"path":"/","roleid":"Administrator","type":"user","ugid":"root@pam","propagate":1},`+
			`{"path":"/vms/100","roleid":"PVEVMUser","type":"group","ugid":"ops","propagate":false}]}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	config := aclTestObjectValue(t, schemaResp.Schema.Type(), nil)
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %s", diagnosticsError(resp.Diagnostics))
	}
	var data pveAclDataSourceModel
	if err := resp.State.Get(ctx, &data); err != nil {
		t.Fatalf("get state: %v", err)
	}
	if len(data.Entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(data.Entries))
	}
	first := data.Entries[0]
	if first.Path.ValueString() != "/" || first.Role.ValueString() != "Administrator" || first.Type.ValueString() != "user" || first.Ugid.ValueString() != "root@pam" || !first.Propagate.ValueBool() {
		t.Fatalf("first = %+v", first)
	}
	second := data.Entries[1]
	if second.Propagate.ValueBool() {
		t.Fatalf("second propagate = %v, want false", second.Propagate)
	}
}
