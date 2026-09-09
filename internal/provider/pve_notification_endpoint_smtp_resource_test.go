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

// notificationEndpointSMTPTestAttrTypes returns the resource attribute types.
func notificationEndpointSMTPTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{"name": tftypes.String, "server": tftypes.String, "from_address": tftypes.String, "username": tftypes.String, "password": tftypes.String, "port": tftypes.Number, "mode": tftypes.String, "mailto": tftypes.List{ElementType: tftypes.String}, "mailto_user": tftypes.List{ElementType: tftypes.String}, "author": tftypes.String, "comment": tftypes.String, "disable": tftypes.Bool}
}

// notificationEndpointSMTPTestRaw builds a resource object value.
func notificationEndpointSMTPTestRaw(vals map[string]tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointSMTPTestAttrTypes()}, vals)
}

// notificationEndpointSMTPTestNullRaw builds a null resource object value.
func notificationEndpointSMTPTestNullRaw() tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationEndpointSMTPTestAttrTypes()}, nil)
}

// notificationEndpointSMTPTestCreateVals returns the CreateVals attribute values.
func notificationEndpointSMTPTestCreateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "smtp1"),
		"server":       tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"from_address": tftypes.NewValue(tftypes.String, "pve@example.com"),
		"username":     tftypes.NewValue(tftypes.String, "mailer"),
		"password":     tftypes.NewValue(tftypes.String, "pw"),
		"port":         tftypes.NewValue(tftypes.Number, 587),
		"mode":         tftypes.NewValue(tftypes.String, "starttls"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"author":       tftypes.NewValue(tftypes.String, nil),
		"comment":      tftypes.NewValue(tftypes.String, "Relay"),
		"disable":      tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	}
}

// notificationEndpointSMTPTestUpdateVals returns the UpdateVals attribute values.
func notificationEndpointSMTPTestUpdateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "smtp1"),
		"server":       tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"from_address": tftypes.NewValue(tftypes.String, "pve@example.com"),
		"username":     tftypes.NewValue(tftypes.String, "mailer"),
		"password":     tftypes.NewValue(tftypes.String, "pw"),
		"port":         tftypes.NewValue(tftypes.Number, 587),
		"mode":         tftypes.NewValue(tftypes.String, "tls"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"author":       tftypes.NewValue(tftypes.String, nil),
		"comment":      tftypes.NewValue(tftypes.String, nil),
		"disable":      tftypes.NewValue(tftypes.Bool, false),
	}
}

// notificationEndpointSMTPTestUpdateStateVals returns the UpdateStateVals attribute values.
func notificationEndpointSMTPTestUpdateStateVals() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "smtp1"),
		"server":       tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"from_address": tftypes.NewValue(tftypes.String, "pve@example.com"),
		"username":     tftypes.NewValue(tftypes.String, "mailer"),
		"password":     tftypes.NewValue(tftypes.String, "pw"),
		"port":         tftypes.NewValue(tftypes.Number, 587),
		"mode":         tftypes.NewValue(tftypes.String, "starttls"),
		"mailto":       tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "ops@example.com")}),
		"mailto_user":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"author":       tftypes.NewValue(tftypes.String, nil),
		"comment":      tftypes.NewValue(tftypes.String, "Relay"),
		"disable":      tftypes.NewValue(tftypes.Bool, false),
	}
}

// TestPveNotificationEndpointSMTP_ResourceMetadataAndSchema covers the
// resource's type name and schema shape.
func TestPveNotificationEndpointSMTP_ResourceMetadataAndSchema(t *testing.T) {
	r := NewPveNotificationEndpointSMTPResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointSmtp {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointSmtp)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "server", "from_address", "username", "password", "port", "mode", "mailto", "mailto_user", "author", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveNotificationEndpointSMTP_ResourceLifecycle runs create, update
// and delete against a fake API and asserts the wire bodies.
func TestPveNotificationEndpointSMTP_ResourceLifecycle(t *testing.T) {
	var updated bool
	r := NewPveNotificationEndpointSMTPResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointSMTPResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := "/cluster/notifications/endpoints/smtp/smtp1"
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/notifications/endpoints/smtp":
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if !strings.Contains(sent, `"name":"smtp1"`) || !strings.Contains(sent, `"server":"smtp.example.com"`) || !strings.Contains(sent, `"from-address":"pve@example.com"`) || !strings.Contains(sent, `"port":587`) || !strings.Contains(sent, `"mode":"starttls"`) {
				t.Fatalf("create body = %s", sent)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == path:
			if !updated {
				_, _ = io.WriteString(w, `{"data":{"name":"smtp1","server":"smtp.example.com","from-address":"pve@example.com","username":"mailer","port":587,"mode":"starttls","mailto":["ops@example.com"],"comment":"Relay","disable":0}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"name":"smtp1","server":"smtp.example.com","from-address":"pve@example.com","username":"mailer","port":587,"mode":"tls","mailto":["ops@example.com"],"disable":0}}`)
		case req.Method == http.MethodPut && req.URL.Path == path:
			body, _ := io.ReadAll(req.Body)
			sent := string(body)
			if got := req.URL.Query().Get("delete"); got != "comment" {
				t.Fatalf("update delete fields = %q, want comment", got)
			}
			if !strings.Contains(sent, `"mode":"tls"`) || !strings.Contains(sent, `"password":"pw"`) {
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
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointSMTPTestNullRaw()},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestCreateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestCreateVals())},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveNotificationEndpointSMTPResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Server.ValueString() != "smtp.example.com" || !created.Port.Equal(types.Int64Value(587)) || created.Mode.ValueString() != "starttls" {
		t.Fatalf("created state = %+v", created)
	}
	if len(created.MailTo.Elements()) != 1 {
		t.Fatalf("mailto = %v", created.MailTo)
	}

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: notificationEndpointSMTPTestNullRaw()},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestUpdateVals())},
		Plan:   tfsdk.Plan{Schema: sch, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestUpdateVals())},
		State:  tfsdk.State{Schema: sch, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestUpdateStateVals())},
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

// TestPveNotificationEndpointSMTP_ResourceRead404Removes verifies Read
// drops the resource from state when the endpoint vanished out of band.
func TestPveNotificationEndpointSMTP_ResourceRead404Removes(t *testing.T) {
	r := NewPveNotificationEndpointSMTPResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationEndpointSMTPResource)
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
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: notificationEndpointSMTPTestRaw(notificationEndpointSMTPTestCreateVals())}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
