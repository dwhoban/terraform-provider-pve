// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPveClusterOptionsResource_MetadataAndSchema covers the resource's type
// name, its singleton id semantics, and the full option attribute set.
func TestPveClusterOptionsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveClusterOptionsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterOptions)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"id", "bwlimit", "consent_text", "console", "crs", "description", "email_from",
		"fencing", "ha", "http_proxy", "keyboard", "language", "location", "mac_prefix",
		"max_workers", "migration", "migration_unsecure", "next_id", "notify",
		"registered_tags", "replication", "tag_style", "u2f", "user_tag_access", "webauthn",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	id, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !id.IsComputed() || id.IsOptional() {
		t.Fatal("id must be computed-only (singleton semantics)")
	}
	console, ok := schemaResp.Schema.Attributes["console"].(schema.StringAttribute)
	if !ok {
		t.Fatal("console attribute is not a StringAttribute")
	}
	if !strings.Contains(console.MarkdownDescription, "xtermjs") ||
		!strings.Contains(console.MarkdownDescription, "Must be one of") {
		t.Fatalf("console description must enumerate the closed set: %q", console.MarkdownDescription)
	}
	maxWorkers, ok := schemaResp.Schema.Attributes["max_workers"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("max_workers attribute is not an Int64Attribute")
	}
	if !strings.Contains(maxWorkers.MarkdownDescription, "Must be at least 1") {
		t.Fatalf("max_workers description must state the range: %q", maxWorkers.MarkdownDescription)
	}
}

// TestPveClusterOptionsModel_UpdateBodyAndDelete drives the model-to-wire
// conversion and the cleared-field diff through the real client: set fields
// travel in the PUT body, cleared fields in the delete query parameter.
func TestPveClusterOptionsModel_UpdateBodyAndDelete(t *testing.T) {
	var sawQuery string
	var captured map[string]any
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.RawQuery
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if err := json.Unmarshal(raw, &captured); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	// State: console, max_workers and ha are managed; the plan clears
	// console and max_workers and changes the ha shutdown policy.
	state := pveClusterOptionsOptionSet{
		Console:    types.StringValue("xtermjs"),
		MaxWorkers: types.Int64Value(4),
		HA:         &pveClusterOptionsHAModel{ShutdownPolicy: types.StringValue("migrate")},
	}
	plan := pveClusterOptionsOptionSet{
		HA: &pveClusterOptionsHAModel{ShutdownPolicy: types.StringValue("conditional")},
	}
	err := client.UpdateClusterOptions(context.Background(), clusterOptionsFromModel(&plan), clusterOptionsDeleteFields(&plan, &state))
	if err != nil {
		t.Fatalf("UpdateClusterOptions: %v", err)
	}
	if got, _ := captured["ha"].(string); got != "shutdown_policy=conditional" {
		t.Fatalf("body[ha] = %q, want shutdown_policy=conditional", got)
	}
	if _, ok := captured["console"]; ok {
		t.Fatal("cleared console must not appear in the body")
	}
	wantQuery := "delete=console%2Cmax_workers"
	if sawQuery != wantQuery {
		t.Fatalf("delete query = %s, want exactly %s", sawQuery, wantQuery)
	}
}

// TestPveClusterOptionsModel_ReadInto applies a fetched option set to the
// model: property strings decode into typed objects, absent options are null.
func TestPveClusterOptionsModel_ReadInto(t *testing.T) {
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"email_from":"ci@example.com",`+
			`"ha":"shutdown_policy=migrate","bwlimit":"move=100,restore=200.5",`+
			`"migration_unsecure":1,"max_workers":4,"next-id":"lower=200,upper=5000"}}`)
	})
	var m pveClusterOptionsOptionSet
	if err := clusterOptionsReadInto(context.Background(), client, &m); err != nil {
		t.Fatalf("clusterOptionsReadInto: %v", err)
	}
	if !m.EmailFrom.Equal(types.StringValue("ci@example.com")) {
		t.Fatalf("email_from = %s", m.EmailFrom)
	}
	if m.HA == nil || !m.HA.ShutdownPolicy.Equal(types.StringValue("migrate")) {
		t.Fatalf("ha = %+v", m.HA)
	}
	if m.BWLimit == nil || !m.BWLimit.Move.Equal(types.Float64Value(100)) ||
		!m.BWLimit.Restore.Equal(types.Float64Value(200.5)) || !m.BWLimit.Clone.IsNull() {
		t.Fatalf("bwlimit = %+v", m.BWLimit)
	}
	if !m.MigrationUnsecure.Equal(types.BoolValue(true)) {
		t.Fatalf("migration_unsecure = %s", m.MigrationUnsecure)
	}
	if !m.MaxWorkers.Equal(types.Int64Value(4)) {
		t.Fatalf("max_workers = %s", m.MaxWorkers)
	}
	if m.NextID == nil || !m.NextID.Lower.Equal(types.Int64Value(200)) ||
		!m.NextID.Upper.Equal(types.Int64Value(5000)) {
		t.Fatalf("next_id = %+v", m.NextID)
	}
	if !m.Console.IsNull() {
		t.Fatalf("console = %s, want null", m.Console)
	}
	if m.CRS != nil {
		t.Fatal("absent crs should be nil")
	}
}

// TestPveClusterOptionsImportID pins the singleton import semantics: any
// import ID resolves to id "cluster" via ImportState.
func TestPveClusterOptionsImportID(t *testing.T) {
	if pveClusterOptionsID != "cluster" {
		t.Fatalf("pveClusterOptionsID = %q, want cluster", pveClusterOptionsID)
	}
}
