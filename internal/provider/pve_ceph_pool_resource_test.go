// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveCephPoolResource_MetadataAndSchema covers the pool resource's
// type name and schema shape.
func TestPveCephPoolResource_MetadataAndSchema(t *testing.T) {
	r := NewPveCephPoolResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephPool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephPool)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "name", "size", "min_size", "pg_num", "pg_num_min", "pg_autoscale_mode", "application", "crush_rule", "target_size", "target_size_ratio", "add_storages", "erasure_coding", "force_destroy", "remove_storages_on_destroy", "pool_id", "pool_type"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveCephPoolDataSource_SchemaAndMetadata covers the pool data source's
// type name and schema shape.
func TestPveCephPoolDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveCephPoolDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephPool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephPool)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "name", "pool_id", "pool_type", "size", "min_size", "pg_num", "pg_num_min", "pg_num_final", "pg_autoscale_mode", "crush_rule_id", "crush_rule_name", "target_size", "target_size_ratio", "bytes_used", "percent_used", "application_metadata", "autoscale_status"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// cephPoolAttrTypes returns the full attribute type map for the pool
// resource state.
func cephPoolAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"node":                       tftypes.String,
		"name":                       tftypes.String,
		"size":                       tftypes.Number,
		"min_size":                   tftypes.Number,
		"pg_num":                     tftypes.Number,
		"pg_num_min":                 tftypes.Number,
		"pg_autoscale_mode":          tftypes.String,
		"application":                tftypes.String,
		"crush_rule":                 tftypes.String,
		"target_size":                tftypes.String,
		"target_size_ratio":          tftypes.Number,
		"add_storages":               tftypes.Bool,
		"erasure_coding":             tftypes.String,
		"force_destroy":              tftypes.Bool,
		"remove_storages_on_destroy": tftypes.Bool,
		"pool_id":                    tftypes.Number,
		"pool_type":                  tftypes.String,
	}
}

// cephPoolPlanRaw builds a full plan value for a pool named pool1.
func cephPoolPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"node":                       tftypes.NewValue(tftypes.String, "pve1"),
		"name":                       tftypes.NewValue(tftypes.String, "pool1"),
		"size":                       tftypes.NewValue(tftypes.Number, 3),
		"min_size":                   tftypes.NewValue(tftypes.Number, 2),
		"pg_num":                     tftypes.NewValue(tftypes.Number, 128),
		"pg_num_min":                 tftypes.NewValue(tftypes.Number, nil),
		"pg_autoscale_mode":          tftypes.NewValue(tftypes.String, "warn"),
		"application":                tftypes.NewValue(tftypes.String, nil),
		"crush_rule":                 tftypes.NewValue(tftypes.String, nil),
		"target_size":                tftypes.NewValue(tftypes.String, nil),
		"target_size_ratio":          tftypes.NewValue(tftypes.Number, nil),
		"add_storages":               tftypes.NewValue(tftypes.Bool, nil),
		"erasure_coding":             tftypes.NewValue(tftypes.String, nil),
		"force_destroy":              tftypes.NewValue(tftypes.Bool, nil),
		"remove_storages_on_destroy": tftypes.NewValue(tftypes.Bool, nil),
		"pool_id":                    tftypes.NewValue(tftypes.Number, nil),
		"pool_type":                  tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: cephPoolAttrTypes()}, vals)
}

// TestPveCephPoolResource_CreateAndDelete runs create (POST, task wait,
// listing read-back) and delete (DELETE with the pin defaults) against a
// fake API.
func TestPveCephPoolResource_CreateAndDelete(t *testing.T) {
	exists := false
	deleted := ""
	r := NewPveCephPoolResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveCephPoolResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/nodes/pve1/ceph/pool":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"name":"pool1"`) || !strings.Contains(string(body), `"pg_autoscale_mode":"warn"`) {
				t.Fatalf("create body missing pool settings: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000001:00000001:cephcreatepool:root@pam:"}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/tasks/UPID:pve1:00000001:00000001:cephcreatepool:root@pam:/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/ceph/pool":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"ceph pool not found"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":[{"pool":2,"pool_name":"pool1","size":3,"min_size":2,"pg_num":128,"pg_autoscale_mode":"warn","crush_rule":1,"crush_rule_name":"replicated_rule","type":"replicated"}]}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/nodes/pve1/ceph/pool/pool1":
			deleted = req.URL.RawQuery
			exists = false
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000002:00000002:cephdestroypool:root@pam:"}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/tasks/UPID:pve1:00000002:00000002:cephdestroypool:root@pam:/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})

	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := cephPoolPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: cephPoolAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveCephPoolResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "pool1" || created.PoolID.ValueInt64() != 2 ||
		created.PoolType.ValueString() != "replicated" || created.CrushRule.ValueString() != "replicated_rule" {
		t.Fatalf("created state = %+v", created)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
	if !strings.Contains(deleted, "remove_ecprofile=1") || !strings.Contains(deleted, "force=0") || !strings.Contains(deleted, "remove_storages=0") {
		t.Fatalf("delete query = %q", deleted)
	}
}
