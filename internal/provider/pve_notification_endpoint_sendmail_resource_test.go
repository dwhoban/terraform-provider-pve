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

// notificationEndpointSendmailTestAttrTypes returns the resource attribute types.
func notificationEndpointSendmailTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{"name": tftypes.String, "mailto": tftypes.List{ElementType: tftypes.String}, "mailto_user": tftypes.List{ElementType: tftypes.String}, "from_address": tftypes.String, "author": tftypes.String, "comment": tftypes.String, "disable": tftypes.Bool}
}

// notificationEndpointSendmailTestRaw builds a resource object value.
func notificationEndpointSendmailTestRaw(vals map[string]tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointSendmailTestAttrTypes()}, vals)
}

// notificationEndpointSendmailTestNullRaw builds a null resource object value.
func notificationEndpointSendmailTestNullRaw() tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointSendmailTestAttrTypes()}, nil)
}

// notificationEndpointSendmailTestCreateVals returns the CreateVals attribute values.
func notificationEndpointSendmailTestCreateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "mail1"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"from_address": tftypes.NewValue(tftypes.String, "pve@example.com"),
		"author":       tftypes.NewValue(tftypes.String, "PVE"),
		"comment":      tftypes.NewValue(tftypes.String, "Alerts"),
		"disable":      tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	}
}

// notificationEndpointSendmailTestUpdateVals returns the UpdateVals attribute values.
func notificationEndpointSendmailTestUpdateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "mail1"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"from_address": tftypes.NewValue(tftypes.String, "new@example.com"),
		"author":       tftypes.NewValue(tftypes.String, nil),
		"comment":      tftypes.NewValue(tftypes.String, nil),
		"disable":      tftypes.NewValue(tftypes.Bool, false),
	}
}

// notificationEndpointSendmailTestUpdateStateVals returns the UpdateStateVals attribute values.
func notificationEndpointSendmailTestUpdateStateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "mail1"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"from_address": tftypes.NewValue(tftypes.String, "pve@example.com"),
		"author":       tftypes.NewValue(tftypes.String, "PVE"),
		"comment":      tftypes.NewValue(tftypes.String, "Alerts"),
		"disable":      tftypes.NewValue(tftypes.Bool, false),
	}
}

// TestPveNotificationEndpointSendmail_ResourceMetadataAndSchema covers the
// resource's type name and schema shape.
func TestPveNotificationEndpointSendmail_ResourceMetadataAndSchema(t *testing.T) {
	r := NewPveNotificationEndpointSendmailResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointSendmail {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointSendmail)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "mailto", "mailto_user", "from_address", "author", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveNotificationEndpointSendmail_ResourceLifecycle runs create, update
// and delete against a fake API and asserts the wire bodies.
func TestPveNotificationEndpointSendmail_ResourceLifecycle(t *testing.T) {
	var updated bool
	r := NewPveNotificationEndpointSendmailResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointSendmailResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := "/cluster/notifications/endpoints/sendmail/mail1"
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/notifications/endpoints/sendmail":
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if !strings.Contains(sent, `"name":"mail1"`) || !strings.Contains(sent, `"mailto":["ops@example.com"]`) || !strings.Contains(sent, `"from-address":"pve@example.com"`) || !strings.Contains(sent, `"author":"PVE"`) {
				t.Fatalf("create body = %s", sent)
			}
			if strings.Contains(sent, "disable") {
				t.Fatalf("unknown disable must not be sent, body = %s", sent)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == path:
			if !updated {
				_, _ = io.WriteString(w, `{"data":{"name":"mail1","from-address":"pve@example.com","mailto":["ops@example.com"],"author":"PVE","comment":"Alerts","disable":false}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"name":"mail1","from-address":"new@example.com","mailto":["ops@example.com"],"disable":false}}`)
		case req.Method == http.MethodPut && req.URL.Path == path:
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if got := req.URL.Query().Get("delete"); got != "author,comment" {
				t.Fatalf("update delete fields = %q, want author,comment", got)
			}
			if !strings.Contains(sent, `"from-address":"new@example.com"`) || strings.Contains(sent, `"author"`) {
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
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointSendmailTestNullRaw()},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestCreateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestCreateVals())},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveNotificationEndpointSendmailResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "mail1" || created.FromAddress.ValueString() != "pve@example.com" || created.Author.ValueString() != "PVE" {
		t.Fatalf("created state = %+v", created)
	}
	if created.Disable.ValueBool() {
		t.Fatalf("disable = %v, want false", created.Disable.ValueBool())
	}

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointSendmailTestNullRaw()},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestUpdateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestUpdateVals())},
		State:  tfsdk.State{Schema: sch, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestUpdateStateVals())},
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

// TestPveNotificationEndpointSendmail_ResourceRead404Removes verifies Read
// drops the resource from state when the endpoint vanished out of band.
func TestPveNotificationEndpointSendmail_ResourceRead404Removes(t *testing.T) {
	r := NewPveNotificationEndpointSendmailResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointSendmailResource)
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
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: notificationEndpointSendmailTestRaw(notificationEndpointSendmailTestCreateVals())}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
