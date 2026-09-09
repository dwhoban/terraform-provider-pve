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

// TestPveNotificationMatcherDataSource_MetadataAndSchema covers the matcher
// data source's type name and schema shape.
func TestPveNotificationMatcherDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveNotificationMatcherDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationMatcher {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationMatcher)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "target", "match_field", "match_severity", "match_calendar", "mode", "invert_match", "disable", "comment", "origin", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationMatcherDataSource_Read verifies the single-matcher
// read decode, including the 0/1 boolish encodings and computed-only
// fields (origin, digest).
func TestPveNotificationMatcherDataSource_Read(t *testing.T) {
	d := NewPveNotificationMatcherDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationMatcherDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/notifications/matchers/ops" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"name":"ops","target":["mail-to-root","team-hook"],` +
			`"match-field":["regex:hostname=^pve"],"match-severity":["error"],` +
			`"match-calendar":["sat..sun 02:30"],"mode":"any","invert-match":1,` +
			`"disable":0,"comment":"page ops","origin":"user-created","digest":"abc123"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "ops"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationMatcherDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Name.ValueString() != "ops" || len(got.Target.Elements()) != 2 || got.Mode.ValueString() != "any" {
		t.Fatalf("matcher = %+v", got)
	}
	if !got.InvertMatch.ValueBool() || got.Disable.ValueBool() {
		t.Fatalf("boolish decode wrong: %+v", got)
	}
	if got.Origin.ValueString() != "user-created" || got.Digest.ValueString() != "abc123" || got.Comment.ValueString() != "page ops" {
		t.Fatalf("computed fields wrong: %+v", got)
	}
}
