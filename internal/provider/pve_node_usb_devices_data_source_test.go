// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodeUsbDevicesDataSource_SchemaAndMetadata covers the USB device
// list data source.
func TestPveNodeUsbDevicesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeUsbDevicesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeUsbDevices {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeUsbDevices)
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
	for _, key := range []string{"busnum", "class", "devnum", "level", "port", "vendid", "prodid", "manufacturer", "product", "serial", "speed", "usbpath"} {
		if devices.NestedObject.Attributes[key] == nil {
			t.Fatalf("devices schema missing %s attribute", key)
		}
	}
}
