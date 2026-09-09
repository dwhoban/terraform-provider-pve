// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveVmAgentInfo_SchemaAndMetadata covers the vm_agent_info data
// source's type name and schema shape.
func TestPveVmAgentInfo_SchemaAndMetadata(t *testing.T) {
	d := NewPveVmAgentInfoDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVmAgentInfo {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVmAgentInfo)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["vmid"].IsRequired() {
		t.Fatal("vmid attribute should be Required")
	}
	for _, key := range []string{"id", "version", "hostname", "os_pretty_name", "time", "timezone", "vcpus", "users", "interfaces"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
		if !schemaResp.Schema.Attributes[key].IsComputed() {
			t.Fatalf("%s attribute should be Computed", key)
		}
	}
}
