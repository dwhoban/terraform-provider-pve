// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveContainersDataSource_SchemaAndMetadata covers the per-node LXC
// container index data source.
func TestPveContainersDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveContainersDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainers {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainers)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "containers"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	containers, ok := schemaResp.Schema.Attributes["containers"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("containers attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["containers"])
	}
	for _, key := range []string{
		"vmid", "status", "name", "template", "cpu", "cpus", "mem", "maxmem",
		"maxswap", "disk", "maxdisk", "diskread", "diskwrite", "netin", "netout",
		"uptime", "lock", "tags", "pressure_cpu_some", "pressure_io_some",
		"pressure_io_full", "pressure_memory_some", "pressure_memory_full",
	} {
		if containers.NestedObject.Attributes[key] == nil {
			t.Fatalf("containers schema missing %s attribute", key)
		}
	}
}
