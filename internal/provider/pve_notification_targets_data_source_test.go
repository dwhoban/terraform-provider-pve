// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveNotificationTargetsDataSource_MetadataAndSchema covers the targets
// data source's type name and schema shape.
func TestPveNotificationTargetsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveNotificationTargetsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationTargets {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationTargets)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "targets"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationTargetsDataSource_Read verifies the targets decode,
// covering a built-in (0/1-encoded disable) and a user-created target with
// the comment absent.
func TestPveNotificationTargetsDataSource_Read(t *testing.T) {
	d := NewPveNotificationTargetsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationTargetsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/notifications/targets" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[` +
			`{"name":"mail-to-root","type":"sendmail","origin":"builtin","comment":"Send mails to root","disable":0},` +
			`{"name":"team-hook","type":"webhook","origin":"user-created","disable":false}]}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationTargetsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(got.Targets))
	}
	builtin := got.Targets[0]
	if builtin.Name.ValueString() != "mail-to-root" || builtin.Type.ValueString() != "sendmail" ||
		builtin.Origin.ValueString() != "builtin" || builtin.Disable.ValueBool() ||
		builtin.Comment.ValueString() != "Send mails to root" {
		t.Fatalf("builtin row = %+v", builtin)
	}
	userCreated := got.Targets[1]
	if userCreated.Origin.ValueString() != "user-created" || userCreated.Disable.ValueBool() || !userCreated.Comment.IsNull() {
		t.Fatalf("user-created row = %+v", userCreated)
	}
}
