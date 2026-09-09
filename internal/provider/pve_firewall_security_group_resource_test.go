// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveFirewallSecurityGroupResource_MetadataAndSchema covers the
// security group resource's type name and schema shape.
func TestPveFirewallSecurityGroupResource_MetadataAndSchema(t *testing.T) {
	r := NewPveFirewallSecurityGroupResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallSecurityGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallSecurityGroup)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"group", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["group"].IsRequired() {
		t.Fatal("group attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["comment"].IsOptional() {
		t.Fatal("comment attribute should be Optional")
	}
}

func firewallSecurityGroupAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"group":   tftypes.String,
		"comment": tftypes.String,
	}
}

func firewallSecurityGroupRaw(group, comment string) tftypes.Value {
	vals := map[string]tftypes.Value{
		"group": tftypes.NewValue(tftypes.String, group),
	}
	if comment == "" {
		vals["comment"] = tftypes.NewValue(tftypes.String, nil)
	} else {
		vals["comment"] = tftypes.NewValue(tftypes.String, comment)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: firewallSecurityGroupAttrTypes()}, vals)
}

// TestPveFirewallSecurityGroupResource_CreateAndDelete runs the create
// (POST then read-back listing) and delete paths against a fake API.
func TestPveFirewallSecurityGroupResource_CreateAndDelete(t *testing.T) {
	r := NewPveFirewallSecurityGroupResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallSecurityGroupResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/firewall/groups":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"group":"web"`) || !strings.Contains(string(body), `"comment":"Web rules"`) {
				t.Fatalf("create body missing group/comment: %q", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/groups":
			_, _ = io.WriteString(w, `{"data":[{"group":"web","comment":"Web rules"}]}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/firewall/groups/web":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := firewallSecurityGroupRaw("web", "Web rules")

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: firewallSecurityGroupAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveFirewallSecurityGroupResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Group.ValueString() != "web" || created.Comment.ValueString() != "Web rules" {
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

// TestPveFirewallSecurityGroupResource_UpdateClearsComment verifies the
// pin's update-by-create form: a comment change POSTs with rename set to
// the group's own name and an explicit empty comment to clear.
func TestPveFirewallSecurityGroupResource_UpdateClearsComment(t *testing.T) {
	var updateBody []byte
	r := NewPveFirewallSecurityGroupResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallSecurityGroupResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/firewall/groups":
			updateBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/groups":
			_, _ = io.WriteString(w, `{"data":[{"group":"web"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	planRaw := firewallSecurityGroupRaw("web", "")
	stateRaw := firewallSecurityGroupRaw("web", "Web rules")

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
	var sent map[string]any
	if err := json.Unmarshal(updateBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", updateBody, err)
	}
	if sent["group"] != "web" || sent["rename"] != "web" {
		t.Fatalf("update body = %v (want rename form)", sent)
	}
	comment, ok := sent["comment"]
	if !ok || comment != "" {
		t.Fatalf("update body must carry explicit empty comment, got %v", sent["comment"])
	}
	var updated pveFirewallSecurityGroupResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if !updated.Comment.IsNull() {
		t.Fatalf("comment = %q, want null after clearing", updated.Comment.ValueString())
	}
}

// TestPveFirewallSecurityGroupResource_ReadVanishedRemoves verifies Read
// drops the resource from state when the group is missing from the listing.
func TestPveFirewallSecurityGroupResource_ReadVanishedRemoves(t *testing.T) {
	r := NewPveFirewallSecurityGroupResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallSecurityGroupResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: firewallSecurityGroupRaw("gone", "")}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
