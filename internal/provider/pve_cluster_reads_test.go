// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// pveClusterReadsTestClient spins up a fake PVE API served by handler and
// returns a token-authenticated client pointed at it.
func pveClusterReadsTestClient(t *testing.T, handler http.HandlerFunc) *pveclient.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := pveclient.NewClient(pveclient.Credentials{
		Endpoint: srv.URL,
		Token:    "root@pam!clusterreads=test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestPveClusterResourcesDataSource_SchemaAndMetadata is the schema test
// for the cluster resource index data source.
func TestPveClusterResourcesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveClusterResourcesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterResources {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterResources)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "type", "resources"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	listType, listOK := schemaResp.Schema.Attributes["resources"].GetType().(types.ListType)
	if !listOK {
		t.Fatalf("resources type = %T, want types.ListType", schemaResp.Schema.Attributes["resources"].GetType())
	}
	objectType, objectOK := listType.ElemType.(types.ObjectType)
	if !objectOK {
		t.Fatalf("resources element type = %T, want types.ObjectType", listType.ElementType)
	}
	row := objectType.AttributeTypes()
	for _, key := range []string{"id", "type", "node", "status", "name", "vmid", "cpu", "maxcpu", "mem", "maxmem", "disk", "maxdisk", "uptime", "shared", "template", "storage", "content", "plugintype", "pool", "level", "tags", "hastate", "host_arch", "cgroup_mode", "zone_type", "network", "network_type", "protocol", "sdn", "lock", "memhost", "netin", "netout", "diskread", "diskwrite"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("resources row missing %s attribute", key)
		}
	}
}

// TestPveClusterStatusDataSource_SchemaAndMetadata covers the cluster
// status overview data source.
func TestPveClusterStatusDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveClusterStatusDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterStatus {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterStatus)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "name", "quorate", "nodes_count", "version", "totem", "qdevice", "nodes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveTasksDataSource_SchemaAndMetadata covers the cluster task list.
func TestPveTasksDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveTasksDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveTasks {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveTasks)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "tasks"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveVersionDataSource_SchemaAndMetadata covers the version data
// source.
func TestPveVersionDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveVersionDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVersion {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVersion)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "release", "repoid", "version", "console"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNextIdFunction_MetadataAndDefinition verifies the function name
// (provider functions drop the provider prefix; the call syntax is
// provider::pve::next_id) and its zero-parameter shape.
func TestPveNextIdFunction_MetadataAndDefinition(t *testing.T) {
	f := NewPveNextIdFunction()
	ctx := context.Background()
	metaResp := &function.MetadataResponse{}
	f.Metadata(ctx, function.MetadataRequest{}, metaResp)
	if metaResp.Name != TypeNamePveNextId {
		t.Fatalf("Name = %q, want %q", metaResp.Name, TypeNamePveNextId)
	}
	defResp := &function.DefinitionResponse{}
	f.Definition(ctx, function.DefinitionRequest{}, defResp)
	if len(defResp.Definition.Parameters) != 0 {
		t.Fatalf("Parameters = %d, want 0", len(defResp.Definition.Parameters))
	}
	if defResp.Definition.Return == nil {
		t.Fatal("Return is nil, want StringReturn")
	}
}

// TestPveNextIdFunction_Run drives Run against a fake API and asserts the
// returned string carries the next free VMID.
func TestPveNextIdFunction_Run(t *testing.T) {
	client := pveClusterReadsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/nextid" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":105}`)
	})
	f := &pveNextIdFunction{client: client}
	runResp := &function.RunResponse{
		Result: function.NewResultData(types.StringNull()),
	}
	f.Run(context.Background(), function.RunRequest{
		Arguments: function.NewArgumentsData([]attr.Value{}),
	}, runResp)
	if runResp.Error != nil {
		t.Fatalf("Run error: %s", runResp.Error.Text)
	}
	got, ok := runResp.Result.Value().(types.String)
	if !ok || got.ValueString() != "105" {
		t.Fatalf("result = %v, want String(\"105\")", runResp.Result.Value())
	}
}

// TestPveNextIdFunction_RunWithoutClient asserts a clear error when the
// provider never published a client.
func TestPveNextIdFunction_RunWithoutClient(t *testing.T) {
	f := &pveNextIdFunction{}
	runResp := &function.RunResponse{
		Result: function.NewResultData(types.StringNull()),
	}
	f.Run(context.Background(), function.RunRequest{
		Arguments: function.NewArgumentsData([]attr.Value{}),
	}, runResp)
	if runResp.Error == nil {
		t.Fatal("expected error without a configured client")
	}
}
