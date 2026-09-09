// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveNodeExecute_SchemaAndMetadata covers the node_execute action's
// type name, schema shape, and the mandated root-only warning.
func TestPveNodeExecute_SchemaAndMetadata(t *testing.T) {
	a := NewPveNodeExecuteAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeExecute {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeExecute)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "commands"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
	if !strings.Contains(schemaResp.Schema.MarkdownDescription, "root-only arbitrary command execution") {
		t.Fatal("schema description should carry the root-only command execution warning")
	}
}
