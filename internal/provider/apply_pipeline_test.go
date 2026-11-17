// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

var applyWidgetItemRe = regexp.MustCompile(`^/api2/json/nodes/[^/]+/widgets/[^/]+$`)

// applyResourceTest builds a genericResource whose widget reads answer 404
// (the object is gone) when gone is true, and otherwise defer to fakePVE.
func applyResourceTest(t *testing.T, d *TypeDescriptor, f *fakePVE, gone bool) *genericResource {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gone && r.Method == http.MethodGet && applyWidgetItemRe.MatchString(r.URL.Path) {
			writeErr(w, http.StatusNotFound, "no such widget")
			return
		}
		f.serve(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL:           srv.URL,
		Credentials:   pveclient.Credentials{APIToken: "root@pam!t=test"},
		Insecure:      true,
		RetryMax:      1,
		RetryBaseWait: 1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return &genericResource{engineBase: engineBase{desc: d, client: c}}
}

// A read-back that finds nothing fails Create but is tolerated by Update.
func TestApplyVanishedReadBackPolicy(t *testing.T) {
	ctx := ctxbg()
	d := widgetDescriptor()

	createValues := map[string]attr.Value{
		"node": types.StringValue("node1"), "name": types.StringValue("alpha"),
		"widget_id": types.Int64Unknown(), "size": types.Int64Unknown(),
	}
	r := applyResourceTest(t, d, newFakePVE(), true)
	diags := r.apply(ctx, applyCreate, createValues, nil)
	if !diags.HasError() {
		t.Fatal("Create with a vanished read-back succeeded, want error")
	}
	if got := diags[0].Summary(); got != "Error Reading pve_widget After Create" {
		t.Fatalf("Create summary = %q", got)
	}

	f := newFakePVE()
	f.widgets["101"] = map[string]any{"id": "101", "name": "alpha", "size": 5}
	r = applyResourceTest(t, d, f, true)
	updateValues := map[string]attr.Value{
		"node": types.StringValue("node1"), "widget_id": types.Int64Value(101),
		"name": types.StringValue("beta"), "size": types.Int64Value(5),
	}
	state := map[string]attr.Value{"id": types.StringValue("node1/101")}
	if diags := r.apply(ctx, applyUpdate, updateValues, state); diags.HasError() {
		t.Fatalf("Update with a vanished read-back failed: %v", diags)
	}
	if got, ok := updateValues["id"].(types.String); !ok || got.ValueString() != "node1/101" {
		t.Fatalf("Update id = %v, want node1/101", updateValues["id"])
	}
	if name := f.widgets["101"]["name"]; name != "beta" {
		t.Fatalf("widget name = %v, want beta (the PUT must still happen)", name)
	}
}

// Update takes a computed path identifier the plan leaves unknown from state.
func TestApplyUpdateCarriesComputedPathParamFromState(t *testing.T) {
	d := widgetDescriptor()
	for i := range d.Attributes {
		if d.Attributes[i].Name == "widget_id" {
			d.Attributes[i].Optional = false // computed-only identifier
		}
	}
	f := newFakePVE()
	f.widgets["101"] = map[string]any{"id": "101", "name": "alpha", "size": 5}
	r := applyResourceTest(t, d, f, false)

	values := map[string]attr.Value{
		"node": types.StringValue("node1"), "widget_id": types.Int64Unknown(),
		"name": types.StringValue("beta"), "size": types.Int64Value(5),
	}
	state := map[string]attr.Value{
		"id": types.StringValue("node1/101"), "widget_id": types.Int64Value(101),
	}
	if diags := r.apply(ctxbg(), applyUpdate, values, state); diags.HasError() {
		t.Fatalf("Update: %v", diags)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(strings.Join(f.requests, "\n"), "PUT /nodes/node1/widgets/101") {
		t.Fatalf("PUT did not target the state's widget_id; requests: %v", f.requests)
	}
}
