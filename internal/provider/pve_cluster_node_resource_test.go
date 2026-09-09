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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPveClusterNodeResource_SchemaAndMetadata asserts the full type name and
// the required attribute set of the managed resource.
func TestPveClusterNodeResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveClusterNodeResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterNode {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterNode)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "peer_host", "peer_password", "fingerprint", "nodeid", "votes", "force", "link0", "ip", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["peer_password"].IsSensitive() {
		t.Fatal("peer_password must be sensitive")
	}
}

// TestPveClusterNodeDataSource_SchemaAndMetadata asserts the data source.
func TestPveClusterNodeDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveClusterNodeDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterNode {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterNode)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "nodeid", "votes", "link0", "ip", "fingerprint", "config_digest", "preferred_node", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveClusterNodeResource_DeleteAndReadAgainstFake exercises the delete
// (200 then 404) and read (member present) flows against a fake PVE API.
func TestPveClusterNodeResource_DeleteAndReadAgainstFake(t *testing.T) {
	lastMember := true
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/config/nodes/pve2":
			if !lastMember {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"node is not in cluster"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/config/nodes":
			nodes := []map[string]string{{"node": "pve1"}}
			if lastMember {
				nodes = append(nodes, map[string]string{"node": "pve2"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nodes})
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/config/join":
			_, _ = io.WriteString(w, `{"data":{"config_digest":"d1","preferred_node":"pve1","nodelist":[{"name":"pve1","nodeid":1,"pve_addr":"10.0.0.11","pve_fp":"AA","quorum_votes":1,"ring0_addr":"10.0.0.11"},{"name":"pve2","nodeid":2,"pve_addr":"10.0.0.12","pve_fp":"BB","quorum_votes":1,"ring0_addr":"10.0.0.12"}]}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	r := &pveClusterNodeResource{client: client}

	// readInto populates computed fields for a member node.
	m := &pveClusterNodeResourceModel{Node: types.StringValue("pve2")}
	if err := r.readInto(context.Background(), m); err != nil {
		t.Fatalf("readInto: %v", err)
	}
	if m.Link0.ValueString() != "10.0.0.12" || m.IP.ValueString() != "10.0.0.12" || m.NodeID.ValueInt64() != 2 {
		t.Fatalf("unexpected computed state: %+v", m)
	}

	// RemoveClusterNode surfaces 404 as an error the resource maps to
	// already-absent success via isPVEClientNotFound.
	lastMember = false
	if err := client.RemoveClusterNode(context.Background(), "pve2"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}
