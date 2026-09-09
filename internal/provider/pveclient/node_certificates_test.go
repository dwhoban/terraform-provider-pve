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

// TestClient_UploadNodeCustomCertificate_BodyShape verifies the upload
// payload carries the PEM chain, key, and boolean flags, and that the
// synchronous certificate info response decodes into NodeCertificateInfo.
func TestClient_UploadNodeCustomCertificate_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/certificates/custom" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{
			"filename": "pveproxy-ssl.pem",
			"fingerprint": "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			"issuer": "CN=PVE Cluster CA",
			"notafter": 1893456000,
			"notbefore": 1735689600,
			"pem": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
			"public-key-bits": 2048,
			"public-key-type": "rsa",
			"san": ["pve1.example.com", "127.0.0.1"],
			"subject": "CN=pve1"
		}}`)
	})

	in := UploadNodeCustomCertificateInput{
		Certificates: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
		Key:          "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n",
		Force:        true,
		Restart:      true,
	}
	info, err := c.UploadNodeCustomCertificate(context.Background(), "pve1", in)
	if err != nil {
		t.Fatalf("UploadNodeCustomCertificate: %v", err)
	}
	if captured["certificates"] != in.Certificates {
		t.Fatalf("certificates = %v", captured["certificates"])
	}
	if captured["key"] != in.Key {
		t.Fatalf("key = %v", captured["key"])
	}
	if captured["force"] != true || captured["restart"] != true {
		t.Fatalf("force = %v, restart = %v", captured["force"], captured["restart"])
	}
	if info.Filename != "pveproxy-ssl.pem" || info.Fingerprint == "" || info.Issuer != "CN=PVE Cluster CA" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.NotAfter == nil || *info.NotAfter != 1893456000 || info.NotBefore == nil || *info.NotBefore != 1735689600 {
		t.Fatalf("unexpected validity: %+v", info)
	}
	if info.PublicKeyBits == nil || *info.PublicKeyBits != 2048 || info.PublicKeyType != "rsa" {
		t.Fatalf("unexpected key info: %+v", info)
	}
	if len(info.SAN) != 2 || info.SAN[0] != "pve1.example.com" || info.Subject != "CN=pve1" {
		t.Fatalf("unexpected san/subject: %+v", info)
	}
}

// TestClient_UploadNodeCustomCertificate_OmitsOptionalFields confirms the
// key and boolean flags are omitted from the body when unset, matching the
// endpoint's optional parameters.
func TestClient_UploadNodeCustomCertificate_OmitsOptionalFields(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})

	if _, err := c.UploadNodeCustomCertificate(context.Background(), "pve1", UploadNodeCustomCertificateInput{
		Certificates: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
	}); err != nil {
		t.Fatalf("UploadNodeCustomCertificate: %v", err)
	}
	if _, ok := captured["key"]; ok {
		t.Fatalf("key should be omitted, got %v", captured["key"])
	}
	if _, ok := captured["force"]; ok {
		t.Fatalf("force should be omitted, got %v", captured["force"])
	}
	if _, ok := captured["restart"]; ok {
		t.Fatalf("restart should be omitted, got %v", captured["restart"])
	}
}

// TestClient_DeleteNodeCustomCertificate_QueryFlags confirms the restart
// flag lands on the query string as the 0/1 integer PVE expects and that a
// null response is not an error.
func TestClient_DeleteNodeCustomCertificate_QueryFlags(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/certificates/custom" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("restart"); got != "1" {
			t.Errorf("restart = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})

	if err := c.DeleteNodeCustomCertificate(context.Background(), "pve1", true); err != nil {
		t.Fatalf("DeleteNodeCustomCertificate: %v", err)
	}
}

// TestClient_OrderAcmeCertificate_BodyShape verifies the order POST carries
// force and returns the task UPID.
func TestClient_OrderAcmeCertificate_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/certificates/acme/certificate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})

	upid, err := c.OrderAcmeCertificate(context.Background(), "pve1", true)
	if err != nil {
		t.Fatalf("OrderAcmeCertificate: %v", err)
	}
	if upid != fakeUpid {
		t.Fatalf("upid = %q", upid)
	}
	if captured["force"] != true {
		t.Fatalf("force = %v", captured["force"])
	}
}

// TestClient_OrderAcmeCertificate_OmitsForce confirms a plain order sends
// an empty body (force defaults to false server-side).
func TestClient_OrderAcmeCertificate_OmitsForce(t *testing.T) {
	var body string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = strings.TrimSpace(string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})

	if _, err := c.OrderAcmeCertificate(context.Background(), "pve1", false); err != nil {
		t.Fatalf("OrderAcmeCertificate: %v", err)
	}
	if body != "" && body != "null" {
		t.Fatalf("body = %q, want no parameters", body)
	}
}

// TestClient_RenewAcmeCertificate_BodyShape verifies the renew PUT carries
// force and returns the task UPID.
func TestClient_RenewAcmeCertificate_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/certificates/acme/certificate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})

	upid, err := c.RenewAcmeCertificate(context.Background(), "pve1", true)
	if err != nil {
		t.Fatalf("RenewAcmeCertificate: %v", err)
	}
	if upid != fakeUpid {
		t.Fatalf("upid = %q", upid)
	}
	if captured["force"] != true {
		t.Fatalf("force = %v", captured["force"])
	}
}

// TestClient_RevokeAcmeCertificate_ReturnsUpid covers the revoke DELETE.
func TestClient_RevokeAcmeCertificate_ReturnsUpid(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/certificates/acme/certificate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})

	upid, err := c.RevokeAcmeCertificate(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("RevokeAcmeCertificate: %v", err)
	}
	if upid != fakeUpid {
		t.Fatalf("upid = %q", upid)
	}
}
