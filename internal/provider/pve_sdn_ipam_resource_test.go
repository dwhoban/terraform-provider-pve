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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveSdnIpamResource_MetadataAndSchema covers the IPAM resource's type
// name and schema shape, including the closed type enum.
func TestPveSdnIpamResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnIpamResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnIpam {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnIpam)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"ipam", "type", "url", "token", "fingerprint", "section"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["ipam"].IsRequired() || !schemaResp.Schema.Attributes["type"].IsRequired() {
		t.Fatal("ipam and type attributes should be Required")
	}
	if !schemaResp.Schema.Attributes["token"].IsSensitive() {
		t.Fatal("token attribute should be Sensitive")
	}
	typeDesc := schemaResp.Schema.Attributes["type"].GetMarkdownDescription()
	for _, want := range []string{"`netbox`", "`phpipam`", "`pve`"} {
		if !strings.Contains(typeDesc, want) {
			t.Fatalf("type description should enumerate %s: %q", want, typeDesc)
		}
	}
}

// TestPveSdnIpamResource_CreateAndDelete runs create (POST with the plugin
// type) and delete against a fake API.
func TestPveSdnIpamResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveSdnIpamResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnIpamResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/sdn/ipams":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"type":"netbox"`) || !strings.Contains(string(body), `"ipam":"netbox1"`) || !strings.Contains(string(body), `"token":"tok"`) {
				t.Fatalf("create body missing type/ipam/token: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/ipams/netbox1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such ipam"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"ipam":"netbox1","type":"netbox","url":"https://netbox.example","token":"tok","section":1}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/sdn/ipams/netbox1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Type{
		"ipam":        tftypes.String,
		"type":        tftypes.String,
		"url":         tftypes.String,
		"token":       tftypes.String,
		"fingerprint": tftypes.String,
		"section":     tftypes.Number,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"ipam":        tftypes.NewValue(tftypes.String, "netbox1"),
		"type":        tftypes.NewValue(tftypes.String, "netbox"),
		"url":         tftypes.NewValue(tftypes.String, "https://netbox.example"),
		"token":       tftypes.NewValue(tftypes.String, "tok"),
		"fingerprint": tftypes.NewValue(tftypes.String, nil),
		"section":     tftypes.NewValue(tftypes.Number, 1),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnIpamResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Type.ValueString() != "netbox" || created.Token.ValueString() != "tok" || created.Section.ValueInt64() != 1 {
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

// TestPveSdnIpamDataSource_MetadataAndRead asserts the data source type
// name and the computed status listing.
func TestPveSdnIpamDataSource_MetadataAndRead(t *testing.T) {
	d := NewPveSdnIpamDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnIpam {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnIpam)
	}

	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnIpamDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/ipams/netbox1":
			_, _ = io.WriteString(w, `{"data":{"ipam":"netbox1","type":"netbox","url":"https://netbox.example"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/ipams/netbox1/status":
			_, _ = io.WriteString(w, `{"data":[{"zone":"zone1","subnet":"10.0.0.0-24","info":["10.0.0.5"]}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := map[string]tftypes.Type{
		"ipam":        tftypes.String,
		"type":        tftypes.String,
		"url":         tftypes.String,
		"token":       tftypes.String,
		"fingerprint": tftypes.String,
		"section":     tftypes.Number,
		"status":      tftypes.List{ElementType: tftypes.String},
	}
	config := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"ipam":        tftypes.NewValue(tftypes.String, "netbox1"),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
		"token":       tftypes.NewValue(tftypes.String, nil),
		"fingerprint": tftypes.NewValue(tftypes.String, nil),
		"section":     tftypes.NewValue(tftypes.Number, nil),
		"status":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{}),
	})

	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var state pveSdnIpamDataSourceModel
	if err := readResp.State.Get(ctx, &state); err != nil {
		t.Fatalf("State.Get after read: %v", err)
	}
	if state.Type.ValueString() != "netbox" {
		t.Fatalf("state = %+v", state)
	}
	var status []string
	if diags := state.Status.ElementsAs(ctx, &status, false); diags.HasError() {
		t.Fatalf("ElementsAs: %+v", diags)
	}
	if len(status) != 1 || !strings.Contains(status[0], `"zone":"zone1"`) || !strings.Contains(status[0], `"subnet":"10.0.0.0-24"`) {
		t.Fatalf("status = %+v", status)
	}
}
