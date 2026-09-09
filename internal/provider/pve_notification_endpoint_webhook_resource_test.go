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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// notificationEndpointWebhookTestAttrTypes returns the resource attribute types.
func notificationEndpointWebhookTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{"name": tftypes.String, "url": tftypes.String, "method": tftypes.String, "body": tftypes.String, "headers": tftypes.Map{ElementType: tftypes.String}, "secrets": tftypes.Map{ElementType: tftypes.String}, "comment": tftypes.String, "disable": tftypes.Bool}
}

// notificationEndpointWebhookTestRaw builds a resource object value.
func notificationEndpointWebhookTestRaw(vals map[string]tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointWebhookTestAttrTypes()}, vals)
}

// notificationEndpointWebhookTestNullRaw builds a null resource object value.
func notificationEndpointWebhookTestNullRaw() tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointWebhookTestAttrTypes()}, nil)
}

// notificationEndpointWebhookTestCreateVals returns the CreateVals attribute values.
func notificationEndpointWebhookTestCreateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "hook1"),
		"url":     tftypes.NewValue(tftypes.String, "https://hooks.example.com/pve"),
		"method":  tftypes.NewValue(tftypes.String, "post"),
		"body":    tftypes.NewValue(tftypes.String, "{\"text\":\"{{ title }}\"}"),
		"headers": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"X-Token": tftypes.NewValue(tftypes.String, "abc")}),
		"secrets": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"HMAC": tftypes.NewValue(tftypes.String, "key")}),
		"comment": tftypes.NewValue(tftypes.String, nil),
		"disable": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	}
}

// notificationEndpointWebhookTestUpdateVals returns the UpdateVals attribute values.
func notificationEndpointWebhookTestUpdateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "hook1"),
		"url":     tftypes.NewValue(tftypes.String, "https://hooks.example.com/pve"),
		"method":  tftypes.NewValue(tftypes.String, "put"),
		"body":    tftypes.NewValue(tftypes.String, nil),
		"headers": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"X-Token": tftypes.NewValue(tftypes.String, "abc")}),
		"secrets": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"HMAC": tftypes.NewValue(tftypes.String, "key")}),
		"comment": tftypes.NewValue(tftypes.String, nil),
		"disable": tftypes.NewValue(tftypes.Bool, false),
	}
}

// notificationEndpointWebhookTestUpdateStateVals returns the UpdateStateVals attribute values.
func notificationEndpointWebhookTestUpdateStateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":    tftypes.NewValue(tftypes.String, "hook1"),
		"url":     tftypes.NewValue(tftypes.String, "https://hooks.example.com/pve"),
		"method":  tftypes.NewValue(tftypes.String, "post"),
		"body":    tftypes.NewValue(tftypes.String, "{\"text\":\"{{ title }}\"}"),
		"headers": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"X-Token": tftypes.NewValue(tftypes.String, "abc")}),
		"secrets": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{"HMAC": tftypes.NewValue(tftypes.String, "key")}),
		"comment": tftypes.NewValue(tftypes.String, nil),
		"disable": tftypes.NewValue(tftypes.Bool, false),
	}
}

// TestPveNotificationEndpointWebhook_ResourceMetadataAndSchema covers the
// resource's type name and schema shape.
func TestPveNotificationEndpointWebhook_ResourceMetadataAndSchema(t *testing.T) {
	r := NewPveNotificationEndpointWebhookResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointWebhook {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointWebhook)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "url", "method", "body", "headers", "secrets", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveNotificationEndpointWebhook_ResourceLifecycle runs create, update
// and delete against a fake API and asserts the wire bodies.
func TestPveNotificationEndpointWebhook_ResourceLifecycle(t *testing.T) {
	var updated bool
	r := NewPveNotificationEndpointWebhookResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointWebhookResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := "/cluster/notifications/endpoints/webhook/hook1"
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/notifications/endpoints/webhook":
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if !strings.Contains(sent, `"name":"hook1"`) || !strings.Contains(sent, `"method":"post"`) || !strings.Contains(sent, `"header":["name=X-Token,value=YWJj"]`) || !strings.Contains(sent, `"secret":["name=HMAC,value=a2V5"]`) {
				t.Fatalf("create body = %s", sent)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == path:
			if !updated {
				_, _ = io.WriteString(w, `{"data":{"name":"hook1","url":"https://hooks.example.com/pve","method":"post","body":"e1sidXJsIjoiaG9vazEifQ==","header":["name=X-Token,value=YWJj"],"secret":["name=HMAC,value=a2V5"],"disable":false}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"name":"hook1","url":"https://hooks.example.com/pve","method":"put","header":["name=X-Token,value=YWJj"],"secret":["name=HMAC,value=a2V5"],"disable":false}}`)
		case req.Method == http.MethodPut && req.URL.Path == path:
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if got := req.URL.Query().Get("delete"); got != "body" {
				t.Fatalf("update delete fields = %q, want body", got)
			}
			if !strings.Contains(sent, `"method":"put"`) || strings.Contains(sent, `"body"`) {
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
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointWebhookTestNullRaw()},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestCreateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestCreateVals())},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveNotificationEndpointWebhookResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.URL.ValueString() != "https://hooks.example.com/pve" || created.Method.ValueString() != "post" {
		t.Fatalf("created state = %+v", created)
	}
	hmacVal, hmacOK := created.Secrets.Elements()["HMAC"].(types.String)
	// safetyassert: elements of a string map are always types.String values.
	if !hmacOK || hmacVal.ValueString() != "key" {
		t.Fatalf("secrets = %v", created.Secrets)
	}

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointWebhookTestNullRaw()},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestUpdateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestUpdateVals())},
		State:  tfsdk.State{Schema: sch, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestUpdateStateVals())},
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

// TestPveNotificationEndpointWebhook_ResourceRead404Removes verifies Read
// drops the resource from state when the endpoint vanished out of band.
func TestPveNotificationEndpointWebhook_ResourceRead404Removes(t *testing.T) {
	r := NewPveNotificationEndpointWebhookResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointWebhookResource)
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
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: notificationEndpointWebhookTestRaw(notificationEndpointWebhookTestCreateVals())}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
