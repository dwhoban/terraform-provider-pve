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

// TestPveNotificationMatcherResource_MetadataAndSchema covers the matcher
// resource's type name and schema shape.
func TestPveNotificationMatcherResource_MetadataAndSchema(t *testing.T) {
	r := NewPveNotificationMatcherResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationMatcher {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationMatcher)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "target", "match_field", "match_severity", "match_calendar", "mode", "invert_match", "disable", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// notificationMatcherAttrTypes mirrors the resource model's types.
func notificationMatcherAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":           tftypes.String,
		"target":         tftypes.List{ElementType: tftypes.String},
		"match_field":    tftypes.List{ElementType: tftypes.String},
		"match_severity": tftypes.List{ElementType: tftypes.String},
		"match_calendar": tftypes.List{ElementType: tftypes.String},
		"mode":           tftypes.String,
		"invert_match":   tftypes.Bool,
		"disable":        tftypes.Bool,
		"comment":        tftypes.String,
	}
}

// notificationMatcherRawFrom builds a model value; unset attributes stay null.
func notificationMatcherRawFrom(set map[string]tftypes.Value) tftypes.Value {
	vals := map[string]tftypes.Value{
		"name":           tftypes.NewValue(tftypes.String, nil),
		"target":         tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"match_field":    tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"match_severity": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"match_calendar": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"mode":           tftypes.NewValue(tftypes.String, nil),
		"invert_match":   tftypes.NewValue(tftypes.Bool, nil),
		"disable":        tftypes.NewValue(tftypes.Bool, nil),
		"comment":        tftypes.NewValue(tftypes.String, nil),
	}
	for k, v := range set {
		vals[k] = v
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: notificationMatcherAttrTypes()}, vals)
}

func notificationMatcherList(vals ...string) tftypes.Value {
	elems := make([]tftypes.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, tftypes.NewValue(tftypes.String, v))
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, elems)
}

// TestPveNotificationMatcherResource_CreateUpdateDelete runs create (POST
// then read-back GET), update (PUT with delete query for cleared comment),
// and delete (DELETE + already-absent success) against a fake API.
func TestPveNotificationMatcherResource_CreateUpdateDelete(t *testing.T) {
	exists := false
	r := NewPveNotificationMatcherResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationMatcherResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/notifications/matchers":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"match-field":["exact:severity=error","regex:hostname=^pve"]`) {
				t.Fatalf("create body missing match-field array: %q", body)
			}
			if !strings.Contains(string(body), `"mode":"any"`) {
				t.Fatalf("create body missing mode: %q", body)
			}
			if strings.Contains(string(body), `"disable"`) {
				t.Fatalf("nil disable must be omitted: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/notifications/matchers/ops":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such matcher"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"name":"ops","target":["mail-to-root"],`+
				`"match-field":["exact:severity=error","regex:hostname=^pve"],`+
				`"mode":"any","disable":0,"comment":"page ops","digest":"abc123"}}`)
		case req.Method == http.MethodPut && req.URL.Path == "/cluster/notifications/matchers/ops":
			body, _ := io.ReadAll(req.Body)
			// Plan drops both the comment (null) and disable (null vs the
			// false read back at create); both must travel in the delete
			// query.
			if !strings.Contains(req.URL.RawQuery, "delete=disable%2Ccomment") {
				t.Fatalf("update must clear comment and disable via delete query, got %q", req.URL.RawQuery)
			}
			if !strings.Contains(string(body), `"mode":"any"`) {
				t.Fatalf("update body missing mode: %q", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/notifications/matchers/ops":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	createPlan := notificationMatcherRawFrom(map[string]tftypes.Value{
		"name":        tftypes.NewValue(tftypes.String, "ops"),
		"target":      notificationMatcherList("mail-to-root"),
		"match_field": notificationMatcherList("exact:severity=error", "regex:hostname=^pve"),
		"mode":        tftypes.NewValue(tftypes.String, "any"),
		"comment":     tftypes.NewValue(tftypes.String, "page ops"),
	})
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: notificationMatcherAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: createPlan},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: createPlan},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveNotificationMatcherResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "ops" || len(created.MatchField.Elements()) != 2 || created.Mode.ValueString() != "any" {
		t.Fatalf("created state = %+v", created)
	}
	if created.Disable.ValueBool() || created.Comment.ValueString() != "page ops" {
		t.Fatalf("computed fill wrong after create: %+v", created)
	}

	updatePlan := notificationMatcherRawFrom(map[string]tftypes.Value{
		"name":        tftypes.NewValue(tftypes.String, "ops"),
		"target":      notificationMatcherList("mail-to-root"),
		"match_field": notificationMatcherList("exact:severity=error", "regex:hostname=^pve"),
		"mode":        tftypes.NewValue(tftypes.String, "any"),
	})
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw}}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: updatePlan},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: updatePlan},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveNotificationMatcherResource_Read404Removes verifies Read drops the
// resource from state when the matcher vanished out of band.
func TestPveNotificationMatcherResource_Read404Removes(t *testing.T) {
	r := NewPveNotificationMatcherResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveNotificationMatcherResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such matcher"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: notificationMatcherRawFrom(map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "gone"),
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
