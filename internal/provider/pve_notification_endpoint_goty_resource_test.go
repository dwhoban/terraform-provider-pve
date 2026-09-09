// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// notificationEndpointGotyTestAttrTypes returns the resource attribute types.
func notificationEndpointGotyTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{"name": tftypes.String, "server": tftypes.String, "token": tftypes.String, "comment": tftypes.String, "disable": tftypes.Bool}
}

// notificationEndpointGotyTestRaw builds a resource object value.
func notificationEndpointGotyTestRaw(vals map[string]tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointGotyTestAttrTypes()}, vals)
}

// notificationEndpointGotyTestNullRaw builds a null resource object value.
func notificationEndpointGotyTestNullRaw() tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointGotyTestAttrTypes()}, nil)
}

// notificationEndpointGotyTestCreateVals returns the CreateVals attribute values.
func notificationEndpointGotyTestCreateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "got1"),
		"server":  tftypes.NewValue(tftypes.String, "https://gotify.example.com"),
		"token":   tftypes.NewValue(tftypes.String, "secret-token"),
		"comment": tftypes.NewValue(tftypes.String, "CI"),
		"disable": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	}
}

// notificationEndpointGotyTestUpdateVals returns the UpdateVals attribute values.
func notificationEndpointGotyTestUpdateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "got1"),
		"server":  tftypes.NewValue(tftypes.String, "https://gotify2.example.com"),
		"token":   tftypes.NewValue(tftypes.String, "secret-token"),
		"comment": tftypes.NewValue(tftypes.String, nil),
		"disable": tftypes.NewValue(tftypes.Bool, false),
	}
}

// notificationEndpointGotyTestUpdateStateVals returns the UpdateStateVals attribute values.
func notificationEndpointGotyTestUpdateStateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "got1"),
		"server":  tftypes.NewValue(tftypes.String, "https://gotify.example.com"),
		"token":   tftypes.NewValue(tftypes.String, "secret-token"),
		"comment": tftypes.NewValue(tftypes.String, "CI"),
		"disable": tftypes.NewValue(tftypes.Bool, false),
	}
}

// TestPveNotificationEndpointGoty_ResourceMetadataAndSchema covers the
// resource's type name and schema shape.
func TestPveNotificationEndpointGoty_ResourceMetadataAndSchema(t *testing.T) {
	r := NewPveNotificationEndpointGotyResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointGoty {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointGoty)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "server", "token", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveNotificationEndpointGoty_ResourceLifecycle runs create, update
// and delete against a fake API and asserts the wire bodies.
func TestPveNotificationEndpointGoty_ResourceLifecycle(t *testing.T) {
	var updated bool
	r := NewPveNotificationEndpointGotyResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointGotyResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := "/cluster/notifications/endpoints/gotify/got1"
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/notifications/endpoints/gotify":
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if !strings.Contains(sent, `"name":"got1"`) || !strings.Contains(sent, `"server":"https://gotify.example.com"`) || !strings.Contains(sent, `"token":"secret-token"`) {
				t.Fatalf("create body = %s", sent)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == path:
			if !updated {
				_, _ = io.WriteString(w, `{"data":{"name":"got1","server":"https://gotify.example.com","comment":"CI","disable":false}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"name":"got1","server":"https://gotify2.example.com","disable":false}}`)
		case req.Method == http.MethodPut && req.URL.Path == path:
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if got := req.URL.Query().Get("delete"); got != "comment" {
				t.Fatalf("update delete fields = %q, want comment", got)
			}
			if !strings.Contains(sent, `"server":"https://gotify2.example.com"`) {
				t.Fatalf("update body = %s", sent)
			}
			updated = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodDelete && req.URL.Path == path:
			updated = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointGotyTestNullRaw()},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestCreateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestCreateVals())},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveNotificationEndpointGotyResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Server.ValueString() != "https://gotify.example.com" || created.Token.ValueString() != "secret-token" || created.Comment.ValueString() != "CI" {
		t.Fatalf("created state = %+v", created)
	}

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointGotyTestNullRaw()},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestUpdateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestUpdateVals())},
		State:  tfsdk.State{Schema: sch, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestUpdateStateVals())},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: sch, Raw: updateResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveNotificationEndpointGoty_ResourceRead404Removes verifies Read
// drops the resource from state when the endpoint vanished out of band.
func TestPveNotificationEndpointGoty_ResourceRead404Removes(t *testing.T) {
	r := NewPveNotificationEndpointGotyResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointGotyResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such notification endpoint"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: notificationEndpointGotyTestRaw(notificationEndpointGotyTestCreateVals())}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
