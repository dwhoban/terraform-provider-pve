// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveNodeCertificateResource_SchemaAndMetadata asserts the full type
// name and the resource attribute set.
func TestPveNodeCertificateResource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeCertificateResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	d.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeCertificate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeCertificate)
	}
	schemaResp := &resource.SchemaResponse{}
	d.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "certificates_pem", "private_key", "force", "restart", "fingerprint", "issuer", "subject", "not_after", "not_before", "san", "public_key_type", "public_key_bits"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeCertificateDataSource_SchemaAndMetadata asserts the full type
// name and the data source attribute set.
func TestPveNodeCertificateDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeCertificateDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeCertificate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeCertificate)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "certificates"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
