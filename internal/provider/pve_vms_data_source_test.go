// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveVmsDataSource_SchemaAndMetadata covers the per-node virtual
// machine index data source.
func TestPveVmsDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveVmsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVms {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVms)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "full", "vms"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if schemaResp.Schema.Attributes["node"] != nil && !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatalf("node attribute should be required")
	}
	vms, ok := schemaResp.Schema.Attributes["vms"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("vms attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["vms"])
	}
	for _, key := range []string{
		"cpu", "cpus", "diskread", "diskwrite", "lock", "maxdisk", "maxmem", "mem",
		"memhost", "name", "netin", "netout", "pid",
		"pressurecpufull", "pressurecpusome", "pressureiofull", "pressureiosome",
		"pressurememoryfull", "pressurememorysome",
		"qmpstatus", "running_machine", "running_qemu", "serial", "status", "tags",
		"template", "uptime", "vmid",
	} {
		if vms.NestedObject.Attributes[key] == nil {
			t.Fatalf("vms schema missing %s attribute", key)
		}
	}
}
