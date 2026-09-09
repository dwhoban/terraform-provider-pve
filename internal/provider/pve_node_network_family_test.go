// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// TestPveNodeNetworkLinuxBridge_MetadataAndSchema covers the bridge
// resource's type name and schema shape.
func TestPveNodeNetworkLinuxBridge_MetadataAndSchema(t *testing.T) {
	r := NewPveNodeNetworkLinuxBridgeResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeNetworkLinuxBridge {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeNetworkLinuxBridge)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "iface", "autostart", "cidr", "gateway", "method", "mtu", "comments", "active", "digest", "bridge_ports", "bridge_vids", "bridge_vlan_aware", "bridge_stp", "bridge_fd"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	nodeAttr, ok := schemaResp.Schema.Attributes["node"].(schema.StringAttribute)
	if !ok {
		t.Fatal("node attribute is not a StringAttribute")
	}
	if nodeAttr.PlanModifiers == nil {
		t.Fatal("node attribute should carry plan modifiers (RequiresReplace)")
	}
}

// TestPveNodeNetworkLinuxBond_MetadataAndSchema covers the bond resource.
func TestPveNodeNetworkLinuxBond_MetadataAndSchema(t *testing.T) {
	r := NewPveNodeNetworkLinuxBondResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeNetworkLinuxBond {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeNetworkLinuxBond)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "iface", "slaves", "bond_mode", "bond_primary", "bond_xmit_hash_policy", "active", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["slaves"].IsRequired() {
		t.Fatal("slaves attribute should be Required")
	}
}

// TestPveNodeNetworkVlan_MetadataAndSchema covers the vlan resource.
func TestPveNodeNetworkVlan_MetadataAndSchema(t *testing.T) {
	r := NewPveNodeNetworkVlanResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeNetworkVlan {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeNetworkVlan)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "iface", "vlan_id", "vlan_raw_device", "active", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["vlan_id"].IsRequired() {
		t.Fatal("vlan_id attribute should be Required")
	}
}

// TestNodeNetworkParseImportID covers the `<node>:<iface>` import contract.
func TestNodeNetworkParseImportID(t *testing.T) {
	for _, tc := range []struct {
		id        string
		node      string
		iface     string
		wantError bool
	}{
		{id: "pve1:vmbr0", node: "pve1", iface: "vmbr0"},
		{id: "pve1:bond0", node: "pve1", iface: "bond0"},
		{id: "pve1", wantError: true},
		{id: ":vmbr0", wantError: true},
		{id: "pve1:", wantError: true},
	} {
		node, iface, err := nodeNetworkParseImportID(tc.id)
		if tc.wantError {
			if err == nil {
				t.Fatalf("id %q: expected error", tc.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("id %q: %v", tc.id, err)
		}
		if node != tc.node || iface != tc.iface {
			t.Fatalf("id %q: got (%s,%s), want (%s,%s)", tc.id, node, iface, tc.node, tc.iface)
		}
	}
}

// newNodeNetworkTestClient spins up a fake PVE API served by h and returns a
// client pointed at it. Token auth avoids the /access/ticket exchange.
func newNodeNetworkTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := pveclient.NewClient(pveclient.Credentials{
		Endpoint: srv.URL,
		Token:    "root@pam!test=00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestNodeNetworkApply_ReloadsAndWaits verifies ADR 0001's folded apply:
// PUT /nodes/{node}/network followed by task polling until exit.
func TestNodeNetworkApply_ReloadsAndWaits(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:NETWORKRELOAD:operator:root@pam:"
	var reloadSeen, statusSeen bool
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/nodes/pve1/network":
			reloadSeen = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/nodes/pve1/tasks/"):
			statusSeen = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := nodeNetworkApply(context.Background(), client, "pve1"); err != nil {
		t.Fatalf("nodeNetworkApply: %v", err)
	}
	if !reloadSeen || !statusSeen {
		t.Fatalf("reload called = %v, task status polled = %v; want both true", reloadSeen, statusSeen)
	}
}

// TestNodeNetworkGetChecked_TypeMismatch verifies a resource never silently
// adopts an interface of another type.
func TestNodeNetworkGetChecked_TypeMismatch(t *testing.T) {
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"iface":"eno1","type":"eth"}]}`)
	})
	_, err := nodeNetworkGetChecked(context.Background(), client, "pve1", "eno1", "bridge")
	if err == nil {
		t.Fatal("expected type-mismatch error")
	}
	var typeErr *nodeNetworkTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("error %v is not *nodeNetworkTypeError", err)
	}
	if typeErr.Got != "eth" || typeErr.Want != "bridge" {
		t.Fatalf("mismatch detail: got %q want %q", typeErr.Got, typeErr.Want)
	}
}
