// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveSubscriptionRefresh_SchemaAndMetadata covers the subscription
// refresh action's type name and schema shape.
func TestPveSubscriptionRefresh_SchemaAndMetadata(t *testing.T) {
	a := NewPveSubscriptionRefreshAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSubscriptionRefresh {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSubscriptionRefresh)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["node"] == nil {
		t.Fatal("schema missing node attribute")
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	if schemaResp.Schema.Attributes["force"] == nil {
		t.Fatal("schema missing force attribute")
	}
	if schemaResp.Schema.Attributes["force"].IsRequired() {
		t.Fatal("force attribute should be Optional")
	}
}
