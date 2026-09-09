// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveClusterNodesDataSource_SchemaAndMetadata is the schema test for
// the cluster node index data source.
func TestPveClusterNodesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveClusterNodesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "scaffolding"}, metaResp)
	if metaResp.TypeName != "scaffolding_"+TypeNamePveClusterNodes {
		t.Fatalf("TypeName = %q, want scaffolding_%s", metaResp.TypeName, TypeNamePveClusterNodes)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "status", "nodes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeStatusDataSource_SchemaAndMetadata covers the per-node status.
func TestPveNodeStatusDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeStatusDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "scaffolding"}, metaResp)
	if metaResp.TypeName != "scaffolding_"+TypeNamePveNodeStatus {
		t.Fatalf("TypeName = %q, want scaffolding_%s", metaResp.TypeName, TypeNamePveNodeStatus)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "cpu", "maxcpu", "mem", "maxmem", "uptime", "level", "kernel", "pveversion", "rootfs_total", "rootfs_used", "rootfs_free"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeDisksDataSource_SchemaAndMetadata covers the per-node disks.
func TestPveNodeDisksDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeDisksDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "scaffolding"}, metaResp)
	if metaResp.TypeName != "scaffolding_"+TypeNamePveNodeDisks {
		t.Fatalf("TypeName = %q, want scaffolding_%s", metaResp.TypeName, TypeNamePveNodeDisks)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "include_smart", "wwn", "disks"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeNetworkInterfacesDataSource_SchemaAndMetadata covers the
// per-node network interface list.
func TestPveNodeNetworkInterfacesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeNetworkInterfacesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "scaffolding"}, metaResp)
	if metaResp.TypeName != "scaffolding_"+TypeNamePveNodeNetworkInterfaces {
		t.Fatalf("TypeName = %q, want scaffolding_%s", metaResp.TypeName, TypeNamePveNodeNetworkInterfaces)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "type", "interfaces"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
