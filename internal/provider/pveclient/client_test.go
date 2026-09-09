// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestClient_TokenAuthHeader asserts that an API-token client attaches the
// PVEAPIToken Authorization header to every request.
func TestClient_TokenAuthHeader(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/access/whoami" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"username":"root@pam","realm":"pam"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := NewClient(Credentials{
		Token:    "root@pam!tf=AAAA-BBBB",
		Endpoint: srv.URL,
		Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	w, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if w.Username != "root@pam" {
		t.Fatalf("Whoami.Username = %q, want root@pam", w.Username)
	}
	want := "PVEAPIToken=root@pam!tf=AAAA-BBBB"
	if sawAuth != want {
		t.Fatalf("Authorization = %q, want %q", sawAuth, want)
	}
}

// TestClient_PasswordAuthIssuesTicketAndSendsCookie verifies that
// username/password auth issues a /access/ticket request on first use and
// then sends PVEAuthCookie on subsequent requests.
func TestClient_PasswordAuthIssuesTicketAndSendsCookie(t *testing.T) {
	var (
		ticketCalls int
		otherCalls  int
		sawCookie   string
		sawUsername string
		sawPassword string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/ticket":
			ticketCalls++
			body, _ := io.ReadAll(r.Body)
			form := string(body)
			sawUsername = extractField(form, "username")
			sawPassword = extractField(form, "password")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-ticket-xyz","username":"root@pam","CSRFPreventionToken":"csrf-abc"}}`)
		default:
			otherCalls++
			for _, ck := range r.Cookies() {
				if ck.Name == "PVEAuthCookie" {
					sawCookie = ck.Value
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"username":"root@pam","realm":"pam"}}`)
		}
	}))
	defer srv.Close()

	c, err := NewClient(Credentials{
		Username: "root@pam",
		Password: "hunter2",
		Endpoint: srv.URL,
		Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if ticketCalls != 1 {
		t.Fatalf("ticket calls = %d, want 1", ticketCalls)
	}
	if otherCalls != 1 {
		t.Fatalf("non-ticket calls = %d, want 1", otherCalls)
	}
	if sawUsername != "root@pam" {
		t.Fatalf("ticket username = %q, want root@pam", sawUsername)
	}
	if sawPassword != "hunter2" {
		t.Fatalf("ticket password = %q, want hunter2", sawPassword)
	}
	if sawCookie != "PVE-ticket-xyz" {
		t.Fatalf("PVEAuthCookie = %q, want PVE-ticket-xyz", sawCookie)
	}
}

// TestClient_PasswordAuthSendsCSRFOnPost asserts the CSRFPreventionToken
// header is added for non-GET requests.
func TestClient_PasswordAuthSendsCSRFOnPost(t *testing.T) {
	var (
		ticketCalls int
		postCSRF    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/ticket":
			ticketCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-tk","username":"root@pam","CSRFPreventionToken":"csrf-tok"}}`)
		default:
			postCSRF = r.Header.Get("CSRFPreventionToken")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":null}`)
		}
	}))
	defer srv.Close()

	c, err := NewClient(Credentials{
		Username: "root@pam",
		Password: "hunter2",
		Endpoint: srv.URL,
		Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Do(context.Background(), "POST", "/access/users", map[string]string{"userid": "new@pve"}, nil); err != nil {
		t.Fatalf("Do POST: %v", err)
	}
	if ticketCalls != 1 {
		t.Fatalf("ticket calls = %d, want 1", ticketCalls)
	}
	if postCSRF != "csrf-tok" {
		t.Fatalf("CSRFPreventionToken = %q, want csrf-tok", postCSRF)
	}
}

// TestClient_PasswordAuthOmitsCSRFOnGet confirms GET requests do NOT carry
// the CSRFPreventionToken header (PVE rejects it on reads).
func TestClient_PasswordAuthOmitsCSRFOnGet(t *testing.T) {
	var (
		ticketCalls int
		getCSRF     string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/ticket":
			ticketCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-tk","username":"root@pam","CSRFPreventionToken":"csrf-tok"}}`)
		default:
			getCSRF = r.Header.Get("CSRFPreventionToken")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
		}
	}))
	defer srv.Close()
	c, err := NewClient(Credentials{
		Username: "root@pam", Password: "hunter2", Endpoint: srv.URL, Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if ticketCalls != 1 {
		t.Fatalf("ticket calls = %d, want 1", ticketCalls)
	}
	if getCSRF != "" {
		t.Fatalf("CSRFPreventionToken on GET = %q, want empty", getCSRF)
	}
}

// TestClient_TicketRefreshOnExpiry forces the ticket's expiry into the
// past and confirms a second call refreshes the ticket.
func TestClient_TicketRefreshOnExpiry(t *testing.T) {
	var ticketCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/access/ticket" {
			ticketCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-tk","username":"root@pam","CSRFPreventionToken":"csrf"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
	}))
	defer srv.Close()
	c, err := NewClient(Credentials{
		Username: "root@pam", Password: "hunter2", Endpoint: srv.URL, Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami 1: %v", err)
	}
	c.mu.Lock()
	c.ticketExp = time.Now().Add(-time.Minute)
	c.mu.Unlock()
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami 2: %v", err)
	}
	if ticketCalls != 2 {
		t.Fatalf("ticket calls = %d, want 2 (one refresh)", ticketCalls)
	}
}

// TestClient_OTPForwardedToTicket verifies the otp field is included in the
// /access/ticket request when set.
func TestClient_OTPForwardedToTicket(t *testing.T) {
	var sawOTP string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/access/ticket" {
			body, _ := io.ReadAll(r.Body)
			sawOTP = extractField(string(body), "otp")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"ticket":"PVE-tk","username":"root@pam","CSRFPreventionToken":"csrf"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
	}))
	defer srv.Close()
	c, err := NewClient(Credentials{
		Username: "root@pam", Password: "hunter2", OTP: "123456",
		Endpoint: srv.URL, Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if sawOTP != "123456" {
		t.Fatalf("otp field on ticket request = %q, want 123456", sawOTP)
	}
}

// TestClient_TLSInsecureAcceptsSelfSigned covers the insecure=true flag.
func TestClient_TLSInsecureAcceptsSelfSigned(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
	}))
	defer srv.Close()
	c, err := NewClient(Credentials{
		Token:    "root@pam!tf=abc",
		Endpoint: srv.URL,
		Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami against self-signed server with Insecure=true: %v", err)
	}
}

// TestClient_TLSCustomCA exercises the root_ca PEM path.
func TestClient_TLSCustomCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"username":"root@pam"}}`)
	}))
	defer srv.Close()
	cert := srv.Certificate()
	if cert == nil {
		t.Fatal("httptest server has no certificate")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	c, err := NewClient(Credentials{
		Token:    "root@pam!tf=abc",
		Endpoint: srv.URL,
		RootCA:   string(pemBytes),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami against self-signed server with matching RootCA: %v", err)
	}
}

// TestNewClient_IncompleteRejected guards the defense-in-depth check.
func TestNewClient_IncompleteRejected(t *testing.T) {
	_, err := NewClient(Credentials{Endpoint: "https://pve.example.com:8006/"})
	if err == nil {
		t.Fatal("NewClient with empty token and user/pass returned no error")
	}
}

// TestNewClient_BadEndpointRejected asserts URL parse failures surface.
func TestNewClient_BadEndpointRejected(t *testing.T) {
	_, err := NewClient(Credentials{Token: "x", Endpoint: "://nope"})
	if err == nil {
		t.Fatal("NewClient accepted malformed endpoint")
	}
}

// TestNewClient_RootCAWithoutCertsFails verifies the pool check rejects
// PEM that has no parseable certificates.
func TestNewClient_RootCAWithoutCertsFails(t *testing.T) {
	_, err := NewClient(Credentials{
		Token:    "x",
		Endpoint: "https://pve.example.com:8006/",
		RootCA:   "not-pem",
	})
	if err == nil {
		t.Fatal("NewClient accepted invalid RootCA PEM")
	}
}

// TestAPIError_RendersStatusAndErrors ensures the formatted error names the
// HTTP status and the PVE error string.
func TestAPIError_RendersStatusAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":"permission check failed"}`)
	}))
	defer srv.Close()
	c, err := NewClient(Credentials{Token: "root@pam!tf=abc", Endpoint: srv.URL, Insecure: "true"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = c.Do(context.Background(), "GET", "/access/users", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Do returned non-APIError: %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("error message missing HTTP status: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "permission check failed") {
		t.Fatalf("error message missing PVE error: %s", err.Error())
	}
}

// extractField pulls a key=value pair out of a URL-encoded body for tests,
// URL-decoding the value so callers see the same string the server received
// after parsing.
func extractField(body, key string) string {
	for _, kv := range strings.Split(body, "&") {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			if decoded, err := url.QueryUnescape(parts[1]); err == nil {
				return decoded
			}
			return parts[1]
		}
	}
	return ""
}
