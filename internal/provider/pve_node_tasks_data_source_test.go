// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodeTasksDataSource_SchemaAndMetadata covers the finished task list
// data source.
func TestPveNodeTasksDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeTasksDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeTasks {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeTasks)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "statusfilter", "limit", "since", "until", "typefilter", "userfilter", "tasks"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	tasks, ok := schemaResp.Schema.Attributes["tasks"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("tasks attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["tasks"])
	}
	for _, key := range []string{"upid", "id", "node", "type", "user", "status", "starttime", "endtime", "pid", "pstart"} {
		if tasks.NestedObject.Attributes[key] == nil {
			t.Fatalf("tasks schema missing %s attribute", key)
		}
	}
}
