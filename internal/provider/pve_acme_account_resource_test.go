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

// TestPveAcmeAccountResource_MetadataAndSchema covers the account
// resource's type name and schema shape.
func TestPveAcmeAccountResource_MetadataAndSchema(t *testing.T) {
	r := NewPveAcmeAccountResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcmeAccount {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcmeAccount)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "contact", "directory", "tos_url", "eab_kid", "eab_hmac_key", "account_url", "tos"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["account_url"].IsComputed() {
		t.Fatal("account_url attribute should be Computed")
	}
}

// acmeAccountAttrTypes returns the plan/raw object type of the model.
func acmeAccountAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":         tftypes.String,
		"contact":      tftypes.List{ElementType: tftypes.String},
		"directory":    tftypes.String,
		"tos_url":      tftypes.String,
		"eab_kid":      tftypes.String,
		"eab_hmac_key": tftypes.String,
		"account_url":  tftypes.String,
		"tos":          tftypes.String,
	}
}

// acmeAccountPlanRaw builds a plan with the required attributes set.
func acmeAccountPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "default"),
		"contact":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "mailto:ops@example.com")}),
		"directory":    tftypes.NewValue(tftypes.String, nil),
		"tos_url":      tftypes.NewValue(tftypes.String, nil),
		"eab_kid":      tftypes.NewValue(tftypes.String, nil),
		"eab_hmac_key": tftypes.NewValue(tftypes.String, nil),
		"account_url":  tftypes.NewValue(tftypes.String, nil),
		"tos":          tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: acmeAccountAttrTypes()}, vals)
}

// TestPveAcmeAccountResource_CreateAndDelete runs create (POST returning a
// UPID plus task wait) and delete (DELETE returning a UPID) against a fake
// API, verifying the contact list is sent joined.
func TestPveAcmeAccountResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveAcmeAccountResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAcmeAccountResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/acme/account":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"contact":"mailto:ops@example.com"`) {
				t.Fatalf("create body missing joined contact: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000001:abcdef01:acme:root@pam:"}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/acme/account/default":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such account"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"location":"https://acme.example/acct/1","directory":"https://acme.example/directory","tos":"https://acme.example/tos"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/acme/account/default":
			exists = false
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000002:abcdef02:acme:root@pam:"}`)
		case strings.HasPrefix(req.URL.Path, "/nodes/pve1/tasks/"):
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Logf("unhandled request: %s %s", req.Method, req.URL.Path)
			_, _ = io.WriteString(w, `{"data":null}`)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := acmeAccountPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: acmeAccountAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveAcmeAccountResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "default" || created.AccountURL.ValueString() != "https://acme.example/acct/1" {
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

// TestPveAcmeAccountResource_Read404Removes verifies Read drops the
// resource from state when the account vanished out of band.
func TestPveAcmeAccountResource_Read404Removes(t *testing.T) {
	r := NewPveAcmeAccountResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAcmeAccountResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such account"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: acmeAccountPlanRaw()}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
