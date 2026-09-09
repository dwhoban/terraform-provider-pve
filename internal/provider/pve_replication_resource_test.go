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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveReplicationResource_MetadataAndSchema covers the replication
// resource's type name and schema shape.
func TestPveReplicationResource_MetadataAndSchema(t *testing.T) {
	r := NewPveReplicationResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveReplication {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveReplication)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "target", "schedule", "rate", "comment", "disable", "type", "guest", "jobnum", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["target"].IsRequired() {
		t.Fatal("target attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["rate"].IsOptional() {
		t.Fatal("rate attribute should be Optional")
	}
	if !schemaResp.Schema.Attributes["schedule"].IsOptional() || !schemaResp.Schema.Attributes["schedule"].IsComputed() {
		t.Fatal("schedule attribute should be Optional+Computed")
	}
	if !schemaResp.Schema.Attributes["type"].IsComputed() || !schemaResp.Schema.Attributes["guest"].IsComputed() {
		t.Fatal("type and guest attributes should be Computed")
	}
}

// replicationResourceAttrTypes maps the resource schema's attribute names
// to their Terraform types.
func replicationResourceAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":       tftypes.String,
		"target":   tftypes.String,
		"schedule": tftypes.String,
		"comment":  tftypes.String,
		"disable":  tftypes.Bool,
		"type":     tftypes.String,
		"rate":     tftypes.Number,
		"guest":    tftypes.Number,
		"jobnum":   tftypes.Number,
		"digest":   tftypes.String,
	}
}

// replicationResourceRaw builds a resource object value setting the given
// attributes and leaving every other attribute null.
func replicationResourceRaw(set map[string]tftypes.Value) tftypes.Value {
	attrTypes := replicationResourceAttrTypes()
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		if v, ok := set[name]; ok {
			vals[name] = v
			continue
		}
		vals[name] = tftypes.NewValue(at, nil)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveReplicationResource_CreateAndDelete runs the create (POST then
// read-back GET) and delete paths against a fake API.
func TestPveReplicationResource_CreateAndDelete(t *testing.T) {
	r := NewPveReplicationResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveReplicationResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/replication":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"id":"100-0"`) || !strings.Contains(string(body), `"target":"pve2"`) || !strings.Contains(string(body), `"type":"local"`) {
				t.Fatalf("create body missing id/target/type: %q", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":{"id":"100-0","type":"local","target":"pve2","guest":100,`+
				`"jobnum":0,"schedule":"*/15","rate":50.5,"disable":1,"comment":"dr site","digest":"abc123"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := replicationResourceRaw(map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "100-0"),
		"target":   tftypes.NewValue(tftypes.String, "pve2"),
		"schedule": tftypes.NewValue(tftypes.String, "*/15"),
		"rate":     tftypes.NewValue(tftypes.Number, 50.5),
		"comment":  tftypes.NewValue(tftypes.String, "dr site"),
		"disable":  tftypes.NewValue(tftypes.Bool, true),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: replicationResourceAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveReplicationResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "100-0" || created.Target.ValueString() != "pve2" || created.Type.ValueString() != "local" {
		t.Fatalf("created state = %+v", created)
	}
	if created.Guest.ValueInt64() != 100 || created.JobNum.ValueInt64() != 0 || created.Digest.ValueString() != "abc123" {
		t.Fatalf("created state = %+v", created)
	}
	if !created.Disable.ValueBool() || created.Rate.ValueFloat64() != 50.5 || created.Comment.ValueString() != "dr site" {
		t.Fatalf("created state = %+v", created)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveReplicationResource_Read404Removes verifies Read drops the
// resource from state when the job vanished out of band.
func TestPveReplicationResource_Read404Removes(t *testing.T) {
	r := NewPveReplicationResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveReplicationResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such replication job"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: replicationResourceRaw(map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, "199-0"),
		"target": tftypes.NewValue(tftypes.String, "pve2"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}

// TestPveReplicationResource_UpdateClearsRemovedRate verifies the update
// diff: a rate removed from configuration travels in the PVE delete
// parameter rather than as a zero value.
func TestPveReplicationResource_UpdateClearsRemovedRate(t *testing.T) {
	r := NewPveReplicationResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveReplicationResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var updateBody []byte
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/cluster/replication/100-0":
			updateBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":{"id":"100-0","type":"local","target":"pve2","guest":100,`+
				`"jobnum":0,"schedule":"*/30","digest":"def456"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	stateRaw := replicationResourceRaw(map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "100-0"),
		"target":   tftypes.NewValue(tftypes.String, "pve2"),
		"schedule": tftypes.NewValue(tftypes.String, "*/15"),
		"rate":     tftypes.NewValue(tftypes.Number, 50.5),
		"type":     tftypes.NewValue(tftypes.String, "local"),
		"guest":    tftypes.NewValue(tftypes.Number, 100),
		"jobnum":   tftypes.NewValue(tftypes.Number, 0),
	})
	planRaw := replicationResourceRaw(map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "100-0"),
		"target":   tftypes.NewValue(tftypes.String, "pve2"),
		"schedule": tftypes.NewValue(tftypes.String, "*/30"),
		"type":     tftypes.NewValue(tftypes.String, "local"),
		"guest":    tftypes.NewValue(tftypes.Number, 100),
		"jobnum":   tftypes.NewValue(tftypes.Number, 0),
	})

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if !strings.Contains(string(updateBody), `"delete":"rate"`) {
		t.Fatalf("update body = %q, want delete:rate", updateBody)
	}
	if strings.Contains(string(updateBody), `"rate":`) {
		t.Fatalf("update body = %q, cleared rate must not travel as a value", updateBody)
	}
	var updated pveReplicationResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if !updated.Rate.IsNull() {
		t.Fatalf("rate should read back null, got %v", updated.Rate)
	}
	if updated.Schedule.ValueString() != "*/30" {
		t.Fatalf("schedule = %q, want */30", updated.Schedule.ValueString())
	}
}
