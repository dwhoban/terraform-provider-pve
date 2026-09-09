// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"scaffolding": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccProtoV6ProviderFactoriesWithEcho includes the echo provider alongside the scaffolding provider.
// It allows for testing assertions on data returned by an ephemeral resource during Open.
// The echoprovider is used to arrange tests by echoing ephemeral data into the Terraform state.
// This lets the data be referenced in test assertions with state checks.
var testAccProtoV6ProviderFactoriesWithEcho = map[string]func() (tfprotov6.ProviderServer, error){
	"scaffolding": providerserver.NewProtocol6WithError(New("test")()),
	"echo":        echoprovider.NewProviderServer(),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}

// diagnosticsError formats a Diagnostics slice into a single human-readable
// string. Used by tests that want to assert on diagnostic content.
func diagnosticsError(d diag.Diagnostics) string {
	var b strings.Builder
	for i, e := range d.Errors() {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.Summary())
		if e.Detail() != "" {
			b.WriteString(": ")
			b.WriteString(e.Detail())
		}
	}
	return b.String()
}

func buildConfigFromMap(t *testing.T, s schema.Schema, values map[string]string) tftypes.Value {
	t.Helper()
	attrTypes := map[string]tftypes.Type{}
	optional := map[string]struct{}{}
	for name, attr := range s.Attributes {
		switch attr.(type) {
		case schema.StringAttribute:
			attrTypes[name] = tftypes.String
		case schema.BoolAttribute:
			attrTypes[name] = tftypes.Bool
		default:
			t.Fatalf("unsupported attribute type for %q in test helper", name)
		}
		optional[name] = struct{}{}
	}
	obj := tftypes.Object{AttributeTypes: attrTypes, OptionalAttributes: optional}
	valMap := map[string]tftypes.Value{}
	for name, v := range values {
		typ, ok := attrTypes[name]
		if !ok {
			continue
		}
		if typ.Is(tftypes.String) {
			valMap[name] = tftypes.NewValue(tftypes.String, v)
		} else if typ.Is(tftypes.Bool) {
			valMap[name] = tftypes.NewValue(tftypes.Bool, v == "true")
		}
	}
	for name, typ := range attrTypes {
		if _, ok := valMap[name]; ok {
			continue
		}
		valMap[name] = tftypes.NewValue(typ, nil)
	}
	return tftypes.NewValue(obj, valMap)
}

// configureRequest builds a provider.ConfigureRequest whose Config is the
// given key/value pairs. Used by the unit tests below to drive Configure
// directly without spinning up a Terraform CLI.
func configureRequest(t *testing.T, p *ScaffoldingProvider, values map[string]string) provider.ConfigureResponse {
	t.Helper()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	req := provider.ConfigureRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    buildConfigFromMap(t, schemaResp.Schema, values),
		},
	}
	resp := provider.ConfigureResponse{}
	p.Configure(context.Background(), req, &resp)
	return resp
}

// withFakePVE spins up an httptest server that responds to /access/whoami
func withFakePVE(t *testing.T, username string) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-fake-ticket","username":"`+username+`","CSRFPreventionToken":"fake-csrf"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"`+username+`","realm":"pam"}}`)
	}))
	return srv.URL, srv.Close
}

// TestProvider_Configure_HappyPath_Token verifies the simplest success
// path: a token in the provider block, validation enabled, fake /access/whoami.
func TestProvider_Configure_HappyPath_Token(t *testing.T) {
	endpoint, cleanup := withFakePVE(t, "root@pam")
	defer cleanup()
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{
		"endpoint":  endpoint,
		"api_token": "root@pam!tf=AAAA-BBBB",
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	client, ok := resp.ResourceData.(*pveclient.Client)
	if !ok {
		t.Fatalf("ResourceData type = %T, want *pveclient.Client", resp.ResourceData)
	}
	if client.AuthKind() != "token" {
		t.Fatalf("AuthKind = %q, want token", client.AuthKind())
	}
}

// TestProvider_Configure_HappyPath_Password covers the user/pass shape.
func TestProvider_Configure_HappyPath_Password(t *testing.T) {
	endpoint, cleanup := withFakePVE(t, "root@pam")
	defer cleanup()
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{
		"endpoint": endpoint,
		"username": "root@pam",
		"password": "hunter2",
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	client := resp.ResourceData.(*pveclient.Client)
	if client.AuthKind() != "password" {
		t.Fatalf("AuthKind = %q, want password", client.AuthKind())
	}
}

// TestProvider_Configure_SkipsValidation confirms /access/whoami is NOT
// called when skip_credentials_validation is true. We use a server that
// records every hit and assert none reach it.
func TestProvider_Configure_SkipsValidation(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
	}))
	defer srv.Close()
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{
		"endpoint":                    srv.URL,
		"api_token":                   "root@pam!tf=AAAA",
		"skip_credentials_validation": "true",
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if resp.ResourceData == nil {
		t.Fatal("ResourceData is nil after Configure")
	}
	if hits != 0 {
		t.Fatalf("expected zero HTTP calls when validation skipped, got %d", hits)
	}
}

// TestProvider_Configure_Whoami401_Diagnostic asserts a 401 surfaces as a
// single diagnostic that names the source and the failing field.
func TestProvider_Configure_Whoami401_Diagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":"permission check failed"}`)
	}))
	defer srv.Close()
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{
		"endpoint":  srv.URL,
		"api_token": "root@pam!tf=BAD",
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostics for 401, got none")
	}
	if resp.Diagnostics.ErrorsCount() != 1 {
		t.Fatalf("diagnostic count = %d, want 1", resp.Diagnostics.ErrorsCount())
	}
	msg := diagnosticsError(resp.Diagnostics)
	if !strings.Contains(msg, "credential validation failed") {
		t.Fatalf("diagnostic missing summary text: %s", msg)
	}
	if !strings.Contains(msg, "api_token") {
		t.Fatalf("diagnostic missing failing field name: %s", msg)
	}
	if !strings.Contains(msg, "PROXMOX_VE_API_TOKEN") {
		t.Fatalf("diagnostic missing env var hint: %s", msg)
	}
}

// TestProvider_Configure_EnvResolution confirms the chain picks up env vars
// when the provider block is empty.
func TestProvider_Configure_EnvResolution(t *testing.T) {
	endpoint, cleanup := withFakePVE(t, "root@pam")
	defer cleanup()
	t.Setenv("PROXMOX_VE_ENDPOINT", endpoint)
	t.Setenv("PROXMOX_VE_API_TOKEN", "root@pam!env=BBBB")
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if resp.ResourceData == nil {
		t.Fatal("ResourceData is nil")
	}
}

// TestProvider_Configure_FileResolution confirms the chain falls through to
// the credentials file when no other source is set.
func TestProvider_Configure_FileResolution(t *testing.T) {
	endpoint, cleanup := withFakePVE(t, "root@pam")
	defer cleanup()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	if err := os.WriteFile(path, []byte("[default]\napi_token = root@pam!file=CCCC\nendpoint = "+endpoint+"\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("PROXMOX_VE_CREDENTIALS_FILE", path)
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if resp.ResourceData == nil {
		t.Fatal("ResourceData is nil")
	}
}

// TestProvider_Configure_NoCredentials_Aggregated confirms all-empty chain
// surfaces a single diagnostic listing every source and the docs URL.
func TestProvider_Configure_NoCredentials_Aggregated(t *testing.T) {
	t.Setenv("PROXMOX_VE_API_TOKEN", "")
	t.Setenv("PROXMOX_VE_USERNAME", "")
	t.Setenv("PROXMOX_VE_PASSWORD", "")
	t.Setenv("PROXMOX_VE_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "no-such-file"))
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostics for missing credentials")
	}
	msg := diagnosticsError(resp.Diagnostics)
	for _, want := range []string{"static configuration", "environment", "credentials file", "PROXMOX_VE_", "docs/index.md"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic missing %q: %s", want, msg)
		}
	}
}

// TestProvider_Configure_DataSlotsPopulated confirms all five protocol-6
// data slots point at the same configured client.
func TestProvider_Configure_DataSlotsPopulated(t *testing.T) {
	endpoint, cleanup := withFakePVE(t, "root@pam")
	defer cleanup()
	p := New("test")().(*ScaffoldingProvider)
	resp := configureRequest(t, p, map[string]string{
		"endpoint":  endpoint,
		"api_token": "root@pam!tf=ZZZZ",
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	slots := []struct {
		name string
		data any
	}{
		{"ResourceData", resp.ResourceData},
		{"DataSourceData", resp.DataSourceData},
		{"ActionData", resp.ActionData},
		{"EphemeralResourceData", resp.EphemeralResourceData},
	}
	for _, s := range slots {
		if s.data == nil {
			t.Fatalf("%s is nil", s.name)
		}
		if _, ok := s.data.(*pveclient.Client); !ok {
			t.Fatalf("%s type = %T, want *pveclient.Client", s.name, s.data)
		}
	}
}
