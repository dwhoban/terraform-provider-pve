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

// TestPveBackupJobDataSource_MetadataAndSchema covers the single job data
// source's type name and schema shape.
func TestPveBackupJobDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveBackupJobDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveBackupJob {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveBackupJob)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "schedule", "included_volumes", "prune_backups", "next_run"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
}

// TestPveBackupJobDataSource_Read reads a job and its included volume tree
// from a fake API.
func TestPveBackupJobDataSource_Read(t *testing.T) {
	d := NewPveBackupJobDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveBackupJobDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newBackupTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/cluster/backup/daily":
			_, _ = w.Write([]byte(`{"data":{"id":"daily","enabled":1,"schedule":"mon..fri 02:00","storage":"local","mode":"snapshot","compress":"zstd","vmid":"100,101","prune-backups":{"keep-last":3,"keep-daily":7},"next-run":1700000000}}`))
		case "/cluster/backup/daily/included_volumes":
			_, _ = w.Write([]byte(`{"data":{"children":[{"id":100,"type":"qemu","name":"web","children":[{"id":"scsi0","included":true,"name":"local:100/vm-disk","reason":"included by mode snapshot"}]},{"id":101,"type":"unknown"}]}}`))
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "daily"),
	})
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveBackupJobDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.ID.ValueString() != "daily" || !data.Enabled.ValueBool() || data.Mode.ValueString() != "snapshot" {
		t.Fatalf("job = %+v", data)
	}
	if data.PruneBackups == nil || data.PruneBackups.KeepLast.ValueInt64() != 3 {
		t.Fatalf("prune_backups = %+v", data.PruneBackups)
	}
	if data.NextRun.ValueInt64() != 1700000000 {
		t.Fatalf("next_run = %s", data.NextRun)
	}
	if len(data.IncludedVolumes) != 2 {
		t.Fatalf("included_volumes = %+v", data.IncludedVolumes)
	}
	web := data.IncludedVolumes[0]
	if web.VMID.ValueInt64() != 100 || web.Type.ValueString() != "qemu" || len(web.Volumes) != 1 || !web.Volumes[0].Included.ValueBool() {
		t.Fatalf("guest = %+v", web)
	}
	if data.IncludedVolumes[1].Name.ValueString() != "" || len(data.IncludedVolumes[1].Volumes) != 0 {
		t.Fatalf("unknown guest = %+v", data.IncludedVolumes[1])
	}
}
