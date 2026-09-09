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

// TestPveFirewallAliasResource_MetadataAndSchema covers the alias
// resource's type name and schema shape.
func TestPveFirewallAliasResource_MetadataAndSchema(t *testing.T) {
	r := NewPveFirewallAliasResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallAlias {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallAlias)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "cidr", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["cidr"].IsRequired() {
		t.Fatal("cidr attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["comment"].IsOptional() {
		t.Fatal("comment attribute should be Optional")
	}
}

func firewallAliasAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":    tftypes.String,
		"cidr":    tftypes.String,
		"comment": tftypes.String,
	}
}

func firewallAliasRaw(name, cidr, comment string) tftypes.Value {
	vals := map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, name),
		"cidr": tftypes.NewValue(tftypes.String, cidr),
	}
	if comment == "" {
		vals["comment"] = tftypes.NewValue(tftypes.String, nil)
	} else {
		vals["comment"] = tftypes.NewValue(tftypes.String, comment)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: firewallAliasAttrTypes()}, vals)
}

// TestPveFirewallAliasResource_CreateAndDelete runs the create (POST then
// read-back GET) and delete paths against a fake API.
func TestPveFirewallAliasResource_CreateAndDelete(t *testing.T) {
	r := NewPveFirewallAliasResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallAliasResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/firewall/aliases":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"cidr":"203.0.113.0/24"`) || !strings.Contains(string(body), `"name":"office"`) {
				t.Fatalf("create body missing name/cidr: %q", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/aliases/office":
			_, _ = io.WriteString(w, `{"data":{"name":"office","cidr":"203.0.113.0/24","comment":"HQ"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/firewall/aliases/office":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := firewallAliasRaw("office", "203.0.113.0/24", "HQ")

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: firewallAliasAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveFirewallAliasResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "office" || created.Cidr.ValueString() != "203.0.113.0/24" || created.Comment.ValueString() != "HQ" {
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

// TestPveFirewallAliasResource_Read404Removes verifies Read drops the
// resource from state when the alias vanished out of band.
func TestPveFirewallAliasResource_Read404Removes(t *testing.T) {
	r := NewPveFirewallAliasResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallAliasResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such alias"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: firewallAliasRaw("gone", "10.0.0.0/8", "")}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
