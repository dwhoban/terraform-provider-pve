// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveBackupJobsDataSource_MetadataAndSchema covers the job list data
// source's type name and schema shape.
func TestPveBackupJobsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveBackupJobsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveBackupJobs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveBackupJobs)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "jobs", "not_backed_up", "vzdump_defaults"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsOptional() {
		t.Fatal("node attribute should be Optional")
	}
	if !schemaResp.Schema.Attributes["jobs"].IsComputed() {
		t.Fatal("jobs attribute should be Computed")
	}
}

// TestPveBackupJobsDataSource_Read lists jobs, not-backed-up guests, and
// (with the node argument set) vzdump defaults from a fake API.
func TestPveBackupJobsDataSource_Read(t *testing.T) {
	d := NewPveBackupJobsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveBackupJobsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newBackupTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[{"id":"daily","enabled":1,"schedule":"mon..fri 02:00","storage":"local","mode":"snapshot","next-run":1700000000}]}`))
		case "/cluster/backup-info/not-backed-up":
			_, _ = w.Write([]byte(`{"data":[{"vmid":101,"type":"lxc"}]}`))
		case "/nodes/pve1/vzdump/defaults":
			_, _ = w.Write([]byte(`{"data":{"all":1,"compress":"zstd","mode":"snapshot","stdexcludes":0,"stopwait":10,"prune-backups":"keep-all=1"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
	})
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveBackupJobsDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.ID.ValueString() != "pve_backup_jobs" {
		t.Fatalf("id = %s", data.ID)
	}
	if len(data.Jobs) != 1 || data.Jobs[0].ID.ValueString() != "daily" || !data.Jobs[0].Enabled.ValueBool() {
		t.Fatalf("jobs = %+v", data.Jobs)
	}
	if len(data.NotBackedUp) != 1 || data.NotBackedUp[0].VMID.ValueInt64() != 101 || data.NotBackedUp[0].Type.ValueString() != "lxc" {
		t.Fatalf("not_backed_up = %+v", data.NotBackedUp)
	}
	if data.VzdumpDefaults == nil || !data.VzdumpDefaults.All.ValueBool() || data.VzdumpDefaults.Compress.ValueString() != "zstd" || data.VzdumpDefaults.PruneBackups.ValueString() != "keep-all=1" {
		t.Fatalf("vzdump_defaults = %+v", data.VzdumpDefaults)
	}
}

// TestPveBackupJobsDataSource_ReadWithoutNode verifies vzdump_defaults
// stays null when the optional node argument is absent.
func TestPveBackupJobsDataSource_ReadWithoutNode(t *testing.T) {
	d := NewPveBackupJobsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveBackupJobsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newBackupTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/cluster/backup-info/not-backed-up":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, nil)
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveBackupJobsDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.VzdumpDefaults != nil {
		t.Fatalf("vzdump_defaults should be null without node, got %+v", data.VzdumpDefaults)
	}
}
