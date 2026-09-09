// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveUserTfaUnlock_SchemaAndMetadata covers the TFA unlock action's
// type name and schema shape.
func TestPveUserTfaUnlock_SchemaAndMetadata(t *testing.T) {
	a := NewPveUserTfaUnlockAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveUserTfaUnlock {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveUserTfaUnlock)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["userid"] == nil {
		t.Fatal("schema missing userid attribute")
	}
	if !schemaResp.Schema.Attributes["userid"].IsRequired() {
		t.Fatal("userid attribute should be Required")
	}
}
