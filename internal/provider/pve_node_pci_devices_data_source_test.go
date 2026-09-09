// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodePciDevicesDataSource_SchemaAndMetadata covers the PCI device
// list data source.
func TestPveNodePciDevicesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodePciDevicesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodePciDevices {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodePciDevices)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "devices"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	devices, ok := schemaResp.Schema.Attributes["devices"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("devices attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["devices"])
	}
	for _, key := range []string{"id", "class", "device", "device_name", "vendor", "vendor_name", "subsystem_device", "subsystem_device_name", "subsystem_vendor", "subsystem_vendor_name", "iommugroup", "mdev"} {
		if devices.NestedObject.Attributes[key] == nil {
			t.Fatalf("devices schema missing %s attribute", key)
		}
	}
}
