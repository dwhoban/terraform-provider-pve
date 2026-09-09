// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveAcmeAccountDataSource_Metadata asserts the data source type name.
func TestPveAcmeAccountDataSource_Metadata(t *testing.T) {
	d := NewPveAcmeAccountDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcmeAccount {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcmeAccount)
	}
}
