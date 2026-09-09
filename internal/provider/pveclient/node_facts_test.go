// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestClient_GetNodeCertificates_Decodes covers the certificates/info
// array decode including the hyphenated public-key fields and SAN list.
func TestClient_GetNodeCertificates_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/certificates/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"filename":"pve-root-ca.pem","fingerprint":"AB:CD","issuer":"CN = PVE Root CA","notafter":1893456000,"notbefore":1609459200,"public-key-bits":2048,"public-key-type":"rsa","san":["pve1.local","127.0.0.1"],"subject":"CN = pve1"}]}`)
	})
	certs, err := c.GetNodeCertificates(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("want 1 certificate, got %d", len(certs))
	}
	cert := certs[0]
	if cert.Filename != "pve-root-ca.pem" || cert.Fingerprint != "AB:CD" || cert.Issuer != "CN = PVE Root CA" || cert.Subject != "CN = pve1" {
		t.Fatalf("unexpected names: %+v", cert)
	}
	if cert.NotBefore == nil || *cert.NotBefore != 1609459200 || cert.NotAfter == nil || *cert.NotAfter != 1893456000 {
		t.Fatalf("unexpected validity: %+v", cert)
	}
	if cert.PublicKeyBits == nil || *cert.PublicKeyBits != 2048 || cert.PublicKeyType != "rsa" {
		t.Fatalf("unexpected public key: %+v", cert)
	}
	if len(cert.SAN) != 2 || cert.SAN[0] != "pve1.local" || cert.SAN[1] != "127.0.0.1" {
		t.Fatalf("unexpected san: %+v", cert.SAN)
	}
}

// TestClient_GetNodeVersion_Decodes covers the per-node version endpoint.
func TestClient_GetNodeVersion_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/version" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"release":"8.4","repoid":"adb2f7fe5f78c62f","version":"8.4.1"}}`)
	})
	v, err := c.GetNodeVersion(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeVersion: %v", err)
	}
	if v.Release != "8.4" || v.RepoID != "adb2f7fe5f78c62f" || v.Version != "8.4.1" {
		t.Fatalf("unexpected version: %+v", v)
	}
}

// TestClient_GetNodeHosts_Decodes covers the /etc/hosts read.
func TestClient_GetNodeHosts_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/hosts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"data":"127.0.0.1 localhost.localdomain localhost\n192.168.1.10 pve1.local pve1\n","digest":"b3a9d1"}}`)
	})
	hosts, err := c.GetNodeHosts(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeHosts: %v", err)
	}
	if hosts.Digest != "b3a9d1" {
		t.Fatalf("unexpected digest: %q", hosts.Digest)
	}
	if want := "127.0.0.1 localhost.localdomain localhost\n192.168.1.10 pve1.local pve1\n"; hosts.Data != want {
		t.Fatalf("data = %q, want %q", hosts.Data, want)
	}
}

// TestClient_SetNodeHosts_BodyShape asserts the POST body carries the
// rendered file content plus digest, and omits the digest when empty.
func TestClient_SetNodeHosts_BodyShape(t *testing.T) {
	var captured map[string]json.RawMessage
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/hosts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	data := "127.0.0.1 localhost\n192.168.1.10 pve1.local pve1\n"
	if err := c.SetNodeHosts(context.Background(), "pve1", data, "b3a9d1"); err != nil {
		t.Fatalf("SetNodeHosts: %v", err)
	}
	var gotData string
	if err := json.Unmarshal(captured["data"], &gotData); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if gotData != data {
		t.Fatalf("data = %q, want %q", gotData, data)
	}
	var gotDigest string
	if err := json.Unmarshal(captured["digest"], &gotDigest); err != nil {
		t.Fatalf("decode digest: %v", err)
	}
	if gotDigest != "b3a9d1" {
		t.Fatalf("digest = %q, want b3a9d1", gotDigest)
	}

	captured = nil
	if err := c.SetNodeHosts(context.Background(), "pve1", data, ""); err != nil {
		t.Fatalf("SetNodeHosts without digest: %v", err)
	}
	if _, ok := captured["digest"]; ok {
		t.Fatalf("digest must be omitted when empty, got %s", captured["digest"])
	}
}
