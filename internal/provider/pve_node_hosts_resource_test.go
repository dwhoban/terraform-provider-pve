// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPveNodeHostsResource_SchemaAndMetadata covers the /etc/hosts
// singleton resource.
func TestPveNodeHostsResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveNodeHostsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeHosts {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeHosts)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "entries", "digest", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	entries, ok := schemaResp.Schema.Attributes["entries"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("entries must be a schema.ListNestedAttribute, got %T", schemaResp.Schema.Attributes["entries"])
	}
	for _, key := range []string{"address", "hostnames"} {
		if entries.NestedObject.Attributes[key] == nil {
			t.Fatalf("entries nested object missing %s attribute", key)
		}
	}
}

// TestPveNodeHostsRenderParseRoundTrip pins the file-format contract:
// render produces `<address> <hostname>...` lines and parse recovers the
// entries while skipping blanks and comments.
func TestPveNodeHostsRenderParseRoundTrip(t *testing.T) {
	entries := []pveNodeHostsEntryModel{
		{
			Address:   types.StringValue("127.0.0.1"),
			Hostnames: listStringToTF([]string{"localhost.localdomain", "localhost"}),
		},
		{
			Address:   types.StringValue("192.168.1.10"),
			Hostnames: listStringToTF([]string{"pve1.local", "pve1"}),
		},
	}
	rendered := nodeHostsRender(entries)
	want := "127.0.0.1 localhost.localdomain localhost\n192.168.1.10 pve1.local pve1\n"
	if rendered != want {
		t.Fatalf("rendered = %q, want %q", rendered, want)
	}

	parsed := nodeHostsParse(rendered)
	if len(parsed) != 2 {
		t.Fatalf("parsed %d entries, want 2", len(parsed))
	}
	if parsed[0].Address.ValueString() != "127.0.0.1" {
		t.Fatalf("entry 0 address = %q", parsed[0].Address.ValueString())
	}
	hostnames := listStringFromTF(parsed[1].Hostnames)
	if len(hostnames) != 2 || hostnames[0] != "pve1.local" || hostnames[1] != "pve1" {
		t.Fatalf("entry 1 hostnames = %v", hostnames)
	}

	withNoise := "# a comment\n\n  \n10.0.0.1 node.local node\n"
	parsed = nodeHostsParse(withNoise)
	if len(parsed) != 1 || parsed[0].Address.ValueString() != "10.0.0.1" {
		t.Fatalf("comment/blank lines must be skipped, got %+v", parsed)
	}
	if got := nodeHostsRender(nil); got != "\n" {
		t.Fatalf("empty entries must render as a lone newline, got %q", got)
	}
}
