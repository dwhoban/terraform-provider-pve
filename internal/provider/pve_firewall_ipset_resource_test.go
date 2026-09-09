// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveFirewallIpsetResource_MetadataAndSchema covers the ipset
// resource's type name and schema shape.
func TestPveFirewallIpsetResource_MetadataAndSchema(t *testing.T) {
	r := NewPveFirewallIpsetResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallIpset {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallIpset)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "comment", "cidrs"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["cidrs"].IsOptional() {
		t.Fatal("cidrs attribute should be Optional")
	}
}

func firewallIpsetAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":    tftypes.String,
		"comment": tftypes.String,
		"cidrs":   tftypes.Set{ElementType: tftypes.String},
	}
}

func firewallIpsetRaw(name, comment string, cidrs ...string) tftypes.Value {
	vals := map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, name),
	}
	if comment == "" {
		vals["comment"] = tftypes.NewValue(tftypes.String, nil)
	} else {
		vals["comment"] = tftypes.NewValue(tftypes.String, comment)
	}
	if len(cidrs) == 0 {
		vals["cidrs"] = tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil)
	} else {
		elems := make([]tftypes.Value, 0, len(cidrs))
		for _, c := range cidrs {
			elems = append(elems, tftypes.NewValue(tftypes.String, c))
		}
		vals["cidrs"] = tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elems)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: firewallIpsetAttrTypes()}, vals)
}

// firewallIpsetTestFake serves a stateful fake ipset: POST to the collection
// creates, POST to /{name} adds a member, DELETE /{name}/{cidr} removes one,
// GETs list the set and its members. It records member additions and
// removals and whether a comment update (collection POST) happened.
type firewallIpsetTestFake struct {
	members        map[string]bool
	added          []string
	removed        []string
	commentUpdated bool
}

func (f *firewallIpsetTestFake) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		t.Helper()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/firewall/ipset":
			f.commentUpdated = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/firewall/ipset/mgmt":
			var body struct {
				Cidr string `json:"cidr"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode add-member body: %v", err)
			}
			f.members[body.Cidr] = true
			f.added = append(f.added, body.Cidr)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/ipset":
			_, _ = io.WriteString(w, `{"data":[{"name":"mgmt","comment":"Admin hosts"}]}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/ipset/mgmt":
			cidrs := make([]string, 0, len(f.members))
			for cidr := range f.members {
				cidrs = append(cidrs, cidr)
			}
			sort.Strings(cidrs)
			entries := make([]string, 0, len(cidrs))
			for _, cidr := range cidrs {
				entries = append(entries, fmt.Sprintf(`{"cidr":%q}`, cidr))
			}
			_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(entries, ","))
		case req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/cluster/firewall/ipset/mgmt/"):
			cidr := strings.TrimPrefix(req.URL.Path, "/cluster/firewall/ipset/mgmt/")
			delete(f.members, cidr)
			f.removed = append(f.removed, cidr)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/firewall/ipset/mgmt":
			f.members = map[string]bool{}
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	}
}

// TestPveFirewallIpsetResource_CreateAddsMembers runs create: the set POST
// followed by one member POST per configured CIDR, then the read-back.
func TestPveFirewallIpsetResource_CreateAddsMembers(t *testing.T) {
	fake := &firewallIpsetTestFake{members: map[string]bool{}}
	r := NewPveFirewallIpsetResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallIpsetResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, fake.handler(t))
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := firewallIpsetRaw("mgmt", "Admin hosts", "10.0.0.1", "192.168.1.0/24")

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: firewallIpsetAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveFirewallIpsetResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "mgmt" || created.Comment.ValueString() != "Admin hosts" || len(created.Cidrs.Elements()) != 2 {
		t.Fatalf("created state = %+v", created)
	}
}

// TestPveFirewallIpsetResource_UpdateDiffsMembers verifies the membership
// diff against upstream: added members POST, removed members DELETE, and no
// comment update when the comment did not change.
func TestPveFirewallIpsetResource_UpdateDiffsMembers(t *testing.T) {
	fake := &firewallIpsetTestFake{members: map[string]bool{
		"10.0.0.1":       true,
		"192.168.1.0/24": true,
	}}
	r := NewPveFirewallIpsetResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallIpsetResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, fake.handler(t))
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	stateRaw := firewallIpsetRaw("mgmt", "", "10.0.0.1", "192.168.1.0/24")
	planRaw := firewallIpsetRaw("mgmt", "", "192.168.1.0/24", "10.0.0.2")

	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}, &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	})
	if fake.commentUpdated {
		t.Fatal("comment update must not fire when the comment is unchanged")
	}
	if len(fake.added) != 1 || fake.added[0] != "10.0.0.2" {
		t.Fatalf("added = %v, want [10.0.0.2]", fake.added)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "10.0.0.1" {
		t.Fatalf("removed = %v, want [10.0.0.1]", fake.removed)
	}
}

// TestPveFirewallIpsetResource_Read404Removes verifies Read drops the
// resource from state when the ipset vanished out of band.
func TestPveFirewallIpsetResource_Read404Removes(t *testing.T) {
	r := NewPveFirewallIpsetResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveFirewallIpsetResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such ipset"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: firewallIpsetRaw("gone", "", "10.0.0.1")}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
