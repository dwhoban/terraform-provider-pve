// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAcme_Account_Lifecycle covers the account wire set: register body with
// the joined contact list and hyphenated EAB keys, read decode of the
// location/directory/tos fields, update body, and deactivate.
func TestAcme_Account_Lifecycle(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/acme/account":
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:abcdef01:acmeaccount:root@pam:"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/acme/account/default":
			_, _ = io.WriteString(w, `{"data":{"account":{"status":"valid"},"directory":"https://acme-staging.example/directory","location":"https://acme-staging.example/acct/1","tos":"https://example.com/tos"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/acme/account/default":
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001235:abcdef02:acmeaccount:root@pam:"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/acme/account/default":
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001236:abcdef03:acmeaccount:root@pam:"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	reg := AcmeAccountRegistration{
		Name:       "default",
		Contact:    []string{"mailto:ops@example.com", "mailto:backup@example.com"},
		Directory:  "https://acme-staging.example/directory",
		TosURL:     "https://example.com/tos",
		EabKid:     "kid-1",
		EabHMACKey: "hmac-secret",
	}
	upid, err := c.RegisterAcmeAccount(ctx, reg)
	if err != nil {
		t.Fatalf("RegisterAcmeAccount: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("register did not return a UPID: %q", upid)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("register body %q is not JSON: %v", lastBody, err)
	}
	if sent["contact"] != "mailto:ops@example.com,mailto:backup@example.com" {
		t.Fatalf("register contact = %v", sent["contact"])
	}
	if sent["name"] != "default" || sent["tos_url"] != "https://example.com/tos" || sent["eab-kid"] != "kid-1" || sent["eab-hmac-key"] != "hmac-secret" {
		t.Fatalf("register body = %v", sent)
	}

	acct, err := c.GetAcmeAccount(ctx, "default")
	if err != nil {
		t.Fatalf("GetAcmeAccount: %v", err)
	}
	if acct.Location != "https://acme-staging.example/acct/1" || acct.Directory != "https://acme-staging.example/directory" || acct.Tos != "https://example.com/tos" {
		t.Fatalf("account = %+v", acct)
	}

	upid, err = c.UpdateAcmeAccount(ctx, "default", []string{"mailto:new@example.com"})
	if err != nil {
		t.Fatalf("UpdateAcmeAccount: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("update did not return a UPID: %q", upid)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["contact"] != "mailto:new@example.com" {
		t.Fatalf("update body = %v", sent)
	}

	upid, err = c.DeactivateAcmeAccount(ctx, "default")
	if err != nil {
		t.Fatalf("DeactivateAcmeAccount: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("deactivate did not return a UPID: %q", upid)
	}
}

// TestAcme_Plugins_CRUD covers the plugin wire set: create body with the
// hyphenated validation-delay key and joined node list, list decode with the
// boolish disable flag, read, update with the delete query, and delete.
func TestAcme_Plugins_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/acme/plugins":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/acme/plugins":
			_, _ = io.WriteString(w, `{"data":[{"plugin":"pdns","type":"dns","api":"pdns","data":"a2V5","disable":1,"nodes":"pve1,pve2","validation-delay":60},{"plugin":"standalone","type":"standalone"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/acme/plugins/pdns":
			_, _ = io.WriteString(w, `{"data":{"plugin":"pdns","type":"dns","api":"pdns","data":"a2V5","disable":true,"validation-delay":60,"digest":"d1"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/acme/plugins/pdns":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/acme/plugins/pdns":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	plugin := AcmePlugin{
		Plugin:          "pdns",
		Type:            "dns",
		API:             "pdns",
		Data:            "a2V5",
		Disable:         HABoolPtr(true),
		Nodes:           []string{"pve1", "pve2"},
		ValidationDelay: HAInt64Ptr(60),
	}
	if err := c.CreateAcmePlugin(ctx, plugin); err != nil {
		t.Fatalf("CreateAcmePlugin: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["id"] != "pdns" || sent["type"] != "dns" || sent["api"] != "pdns" || sent["data"] != "a2V5" || sent["disable"] != true || sent["nodes"] != "pve1,pve2" || sent["validation-delay"] != float64(60) {
		t.Fatalf("create body = %v", sent)
	}

	plugins, err := c.ListAcmePlugins(ctx, "dns")
	if err != nil {
		t.Fatalf("ListAcmePlugins: %v", err)
	}
	if !strings.Contains(lastQuery, "type=dns") {
		t.Fatalf("list query = %q, want type=dns", lastQuery)
	}
	if len(plugins) != 2 || plugins[0].Plugin != "pdns" || plugins[1].Type != "standalone" {
		t.Fatalf("plugins = %+v", plugins)
	}
	if plugins[0].Disable == nil || !*plugins[0].Disable {
		t.Fatalf("boolish disable decode = %+v", plugins[0])
	}
	if len(plugins[0].Nodes) != 2 || plugins[0].Nodes[1] != "pve2" {
		t.Fatalf("node list split = %+v", plugins[0].Nodes)
	}

	read, err := c.GetAcmePlugin(ctx, "pdns")
	if err != nil {
		t.Fatalf("GetAcmePlugin: %v", err)
	}
	if read.Digest != "d1" || read.ValidationDelay == nil || *read.ValidationDelay != 60 {
		t.Fatalf("read plugin = %+v", read)
	}

	update := AcmePlugin{ValidationDelay: HAInt64Ptr(120)}
	if err := c.UpdateAcmePlugin(ctx, "pdns", update, []string{"data"}); err != nil {
		t.Fatalf("UpdateAcmePlugin: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=data") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["validation-delay"] != float64(120) {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteAcmePlugin(ctx, "pdns"); err != nil {
		t.Fatalf("DeleteAcmePlugin: %v", err)
	}
}

// TestCustomCPUModels_CRUD covers the custom CPU model wire set: create body
// with the hyphenated keys, list and read decode with the boolish hidden
// flag, update with the delete query, and delete.
func TestCustomCPUModels_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/qemu/custom-cpu-models":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/qemu/custom-cpu-models":
			_, _ = io.WriteString(w, `{"data":[{"cputype":"lab-cpu","reported-model":"Skylake-Client"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/qemu/custom-cpu-models/lab-cpu":
			_, _ = io.WriteString(w, `{"data":{"cputype":"lab-cpu","reported-model":"Skylake-Client","flags":"+pcid;+spec-ctrl","guest-phys-bits":40,"hidden":1,"hv-vendor-id":"LABVND","level":30,"phys-bits":"host","digest":"d1"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/qemu/custom-cpu-models/lab-cpu":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/qemu/custom-cpu-models/lab-cpu":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	model := CustomCPUModel{
		Name:          "lab-cpu",
		ReportedModel: "Skylake-Client",
		Flags:         "+pcid;+spec-ctrl",
		GuestPhysBits: HAInt64Ptr(40),
		Hidden:        HABoolPtr(true),
		HVVendorID:    "LABVND",
		Level:         HAInt64Ptr(30),
		PhysBits:      "host",
	}
	if err := c.CreateCustomCPUModel(ctx, model); err != nil {
		t.Fatalf("CreateCustomCPUModel: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["cputype"] != "lab-cpu" || sent["reported-model"] != "Skylake-Client" || sent["guest-phys-bits"] != float64(40) || sent["hv-vendor-id"] != "LABVND" || sent["level"] != float64(30) || sent["hidden"] != true || sent["phys-bits"] != "host" || sent["flags"] != "+pcid;+spec-ctrl" {
		t.Fatalf("create body = %v", sent)
	}

	models, err := c.ListCustomCPUModels(ctx)
	if err != nil {
		t.Fatalf("ListCustomCPUModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "lab-cpu" || models[0].ReportedModel != "Skylake-Client" {
		t.Fatalf("models = %+v", models)
	}

	read, err := c.GetCustomCPUModel(ctx, "lab-cpu")
	if err != nil {
		t.Fatalf("GetCustomCPUModel: %v", err)
	}
	if read.Digest != "d1" || read.Hidden == nil || !*read.Hidden || read.PhysBits != "host" || read.Flags != "+pcid;+spec-ctrl" {
		t.Fatalf("read model = %+v", read)
	}

	update := CustomCPUModel{ReportedModel: "host"}
	if err := c.UpdateCustomCPUModel(ctx, "lab-cpu", update, []string{"flags", "level"}); err != nil {
		t.Fatalf("UpdateCustomCPUModel: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "flags") || !strings.Contains(lastQuery, "level") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["reported-model"] != "host" {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteCustomCPUModel(ctx, "lab-cpu"); err != nil {
		t.Fatalf("DeleteCustomCPUModel: %v", err)
	}
}
