// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestClusterMetricsList decodes GET /cluster/metrics/server, including the
// 0/1 disable encoding and the string encoding of numeric settings some
// PVE versions emit from status.cfg.
func TestClusterMetricsList(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/metrics/server" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"id":"influx","type":"influxdb","server":"10.0.0.5","port":8089,"disable":0},`+
			`{"id":"graph","type":"graphite","server":"10.0.0.6","port":"2003","disable":1}]}`)
	})
	servers, err := c.ListMetricsServers(context.Background())
	if err != nil {
		t.Fatalf("ListMetricsServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	influx := servers[0]
	if influx.ID != "influx" || influx.Type != "influxdb" || influx.Server != "10.0.0.5" {
		t.Fatalf("unexpected influx row: %+v", influx)
	}
	if influx.Port == nil || *influx.Port != 8089 {
		t.Fatalf("port = %v, want 8089", influx.Port)
	}
	if influx.Disable == nil || *influx.Disable {
		t.Fatalf("disable = %v, want false", influx.Disable)
	}
	graph := servers[1]
	if graph.Type != "graphite" || graph.Server != "10.0.0.6" {
		t.Fatalf("unexpected graphite row: %+v", graph)
	}
	// Numeric settings may arrive as strings from the section config.
	if graph.Port == nil || *graph.Port != 2003 {
		t.Fatalf("port = %v, want 2003 (string encoding)", graph.Port)
	}
	if graph.Disable == nil || !*graph.Disable {
		t.Fatalf("disable = %v, want true", graph.Disable)
	}
}

// TestClusterMetricsCreate verifies POST /cluster/metrics/server/{id} (the
// pin places create on the id leaf) and the wire body: only set fields
// travel, with id and type included.
func TestClusterMetricsCreate(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/metrics/server/influx1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	srv := MetricsServer{
		ID:                "influx1",
		Type:              "influxdb",
		Server:            "influx.example.com",
		Port:              int64Ptr(8089),
		InfluxDBProto:     "http",
		Organization:      "pve",
		Bucket:            "proxmox",
		Token:             "s3cret-token",
		MaxBodySize:       int64Ptr(25000000),
		VerifyCertificate: boolPtr(true),
	}
	if err := c.CreateMetricsServer(context.Background(), srv); err != nil {
		t.Fatalf("CreateMetricsServer: %v", err)
	}
	if sawBody["id"] != "influx1" || sawBody["type"] != "influxdb" || sawBody["server"] != "influx.example.com" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["port"] != float64(8089) || sawBody["influxdbproto"] != "http" {
		t.Fatalf("unexpected port/proto: %v", sawBody)
	}
	if sawBody["organization"] != "pve" || sawBody["bucket"] != "proxmox" || sawBody["token"] != "s3cret-token" {
		t.Fatalf("unexpected influx fields: %v", sawBody)
	}
	if sawBody["max-body-size"] != float64(25000000) || sawBody["verify-certificate"] != true {
		t.Fatalf("unexpected body sizing fields: %v", sawBody)
	}
	// Fields of the other plugin types must never leak into the body.
	for _, key := range []string{"proto", "path", "timeout", "mtu", "api-path-prefix", "otel-path"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent when unset", key)
		}
	}
}

// TestClusterMetricsCreate_MissingRequired confirms the client rejects
// creates missing the pin's required parameters before hitting the wire.
func TestClusterMetricsCreate_MissingRequired(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	if err := c.CreateMetricsServer(context.Background(), MetricsServer{Type: "graphite"}); err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if err := c.CreateMetricsServer(context.Background(), MetricsServer{ID: "g1", Server: "h", Port: int64Ptr(1)}); err == nil {
		t.Fatal("expected error for missing type, got nil")
	}
}

// TestClusterMetricsGet decodes GET /cluster/metrics/server/{id}: the 0/1
// encodings of disable and verify-certificate, absent fields staying nil,
// and the read-only digest.
func TestClusterMetricsGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/metrics/server/influx1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"influx1","type":"influxdb","server":"influx.example.com",`+
			`"port":8089,"influxdbproto":"https","bucket":"proxmox","organization":"pve","token":"abc",`+
			`"verify-certificate":0,"disable":1,"max-body-size":25000000,"digest":"a1b2c3"}}`)
	})
	srv, err := c.GetMetricsServer(context.Background(), "influx1")
	if err != nil {
		t.Fatalf("GetMetricsServer: %v", err)
	}
	if srv.ID != "influx1" || srv.Type != "influxdb" || srv.Server != "influx.example.com" {
		t.Fatalf("unexpected server: %+v", srv)
	}
	if srv.InfluxDBProto != "https" || srv.Bucket != "proxmox" || srv.Organization != "pve" || srv.Token != "abc" {
		t.Fatalf("unexpected influx fields: %+v", srv)
	}
	if srv.VerifyCertificate == nil || *srv.VerifyCertificate {
		t.Fatalf("verify-certificate = %v, want false", srv.VerifyCertificate)
	}
	if srv.Disable == nil || !*srv.Disable {
		t.Fatalf("disable = %v, want true", srv.Disable)
	}
	if srv.MaxBodySize == nil || *srv.MaxBodySize != 25000000 {
		t.Fatalf("max-body-size = %v, want 25000000", srv.MaxBodySize)
	}
	if srv.Digest != "a1b2c3" {
		t.Fatalf("digest = %q, want a1b2c3", srv.Digest)
	}
	// Graphite and OpenTelemetry fields stay nil for an influxdb server.
	if srv.Proto != "" || srv.Path != "" || srv.Timeout != nil || srv.OTelPath != "" || srv.OTelVerifySSL != nil {
		t.Fatalf("foreign type fields decoded: %+v", srv)
	}
}

// TestClusterMetricsGet_OTel covers the opentelemetry field set with the
// string encoding of otel-timeout.
func TestClusterMetricsGet_OTel(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"otel1","type":"opentelemetry","server":"otel.example.com",`+
			`"port":4318,"otel-protocol":"https","otel-path":"/v1/metrics","otel-compression":"gzip",`+
			`"otel-timeout":"5","otel-verify-ssl":1,"otel-headers":"e30=","otel-max-body-size":10000000}}`)
	})
	srv, err := c.GetMetricsServer(context.Background(), "otel1")
	if err != nil {
		t.Fatalf("GetMetricsServer: %v", err)
	}
	if srv.Type != "opentelemetry" || srv.OTelProtocol != "https" || srv.OTelPath != "/v1/metrics" {
		t.Fatalf("unexpected otel fields: %+v", srv)
	}
	if srv.OTelCompression != "gzip" || srv.OTelHeaders != "e30=" {
		t.Fatalf("unexpected otel compression/headers: %+v", srv)
	}
	if srv.OTelTimeout == nil || *srv.OTelTimeout != 5 {
		t.Fatalf("otel-timeout = %v, want 5 (string encoding)", srv.OTelTimeout)
	}
	if srv.OTelVerifySSL == nil || !*srv.OTelVerifySSL {
		t.Fatalf("otel-verify-ssl = %v, want true", srv.OTelVerifySSL)
	}
	if srv.OTelMaxBodySize == nil || *srv.OTelMaxBodySize != 10000000 {
		t.Fatalf("otel-max-body-size = %v, want 10000000", srv.OTelMaxBodySize)
	}
}

// TestClusterMetricsUpdate verifies PUT /cluster/metrics/server/{id}: the
// body carries id (required per the pin) but never type (the plugin type is
// immutable), and the delete slice travels as the `delete` query parameter.
func TestClusterMetricsUpdate(t *testing.T) {
	var sawQuery url.Values
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/metrics/server/graph1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.Query()
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	srv := MetricsServer{
		ID:      "graph1",
		Type:    "graphite",
		Server:  "graphite.example.com",
		Port:    int64Ptr(2003),
		Proto:   "tcp",
		Path:    "proxmox",
		Timeout: int64Ptr(2),
		MTU:     int64Ptr(1500),
	}
	if err := c.UpdateMetricsServer(context.Background(), "graph1", srv, []string{"organization", "token"}); err != nil {
		t.Fatalf("UpdateMetricsServer: %v", err)
	}
	if got := sawQuery.Get("delete"); got != "organization,token" {
		t.Fatalf("delete param = %q, want organization,token", got)
	}
	if sawBody["id"] != "graph1" {
		t.Fatalf("body id = %v, want graph1", sawBody["id"])
	}
	if _, ok := sawBody["type"]; ok {
		t.Fatal("type must not be sent on update (pin's PUT has no type parameter)")
	}
	if sawBody["server"] != "graphite.example.com" || sawBody["port"] != float64(2003) || sawBody["proto"] != "tcp" {
		t.Fatalf("unexpected body scalars: %v", sawBody)
	}
	if sawBody["path"] != "proxmox" || sawBody["timeout"] != float64(2) || sawBody["mtu"] != float64(1500) {
		t.Fatalf("unexpected graphite fields: %v", sawBody)
	}
}

// TestClusterMetricsDelete confirms DELETE /cluster/metrics/server/{id} and
// that a 404 surfaces as *APIError so the resource layer can map it to
// "already absent".
func TestClusterMetricsDelete(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/metrics/server/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such metric server ('gone')\n"}`)
	})
	err := c.DeleteMetricsServer(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing 404: %v", err)
	}
}
