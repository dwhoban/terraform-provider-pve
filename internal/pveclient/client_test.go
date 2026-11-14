// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, creds Credentials, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{
		URL:            srv.URL,
		Credentials:    creds,
		Insecure:       true,
		RequestTimeout: 5 * time.Second,
		RetryMax:       2,
		RetryBaseWait:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestParseAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "default port", in: "https://pve.example.com", want: "https://pve.example.com:8006"},
		{name: "explicit port", in: "https://pve.example.com:8443", want: "https://pve.example.com:8443"},
		{name: "trailing slash", in: "https://pve.example.com/", want: "https://pve.example.com:8006"},
		{name: "missing scheme", in: "pve.example.com", wantErr: "scheme"},
		{name: "http rejected", in: "http://pve.example.com", wantErr: "https"},
		{name: "path rejected", in: "https://pve.example.com/prefix", wantErr: "path"},
		{name: "empty", in: "", wantErr: "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParseAPIURL(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.String() != tt.want {
				t.Errorf("got %q, want %q", u.String(), tt.want)
			}
		})
	}
}

func TestNewClientIncompleteCredentials(t *testing.T) {
	if _, err := NewClient(Config{URL: "https://x:8006", Credentials: Credentials{Username: "root@pam"}}); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestClientTokenAuthHeader(t *testing.T) {
	var gotAuth atomic.Value
	c := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/version" {
			gotAuth.Store(r.Header.Get("Authorization"))
			writeJSON(w, http.StatusOK, `{"data":{"version":"9.2.2","release":"9.2","repoid":"b9984c6d"}}`)
			return
		}
		writeJSON(w, http.StatusNotFound, `{"data":null}`)
	}))

	v, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.Version != "9.2.2" || v.Release != "9.2" || v.Repoid != "b9984c6d" {
		t.Errorf("parsed %+v", v)
	}
	if gotAuth.Load() != "PVEAPIToken=root@pam!t=abc" {
		t.Errorf("Authorization = %v", gotAuth.Load())
	}
}

func TestClientAPIErrorAndUnauthorized(t *testing.T) {
	c := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, `{"data":null,"errors":{"ticket":"invalid"},"message":"401 Permission denied - invalid PVE ticket"}`)
	}))

	_, err := c.GetVersion(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || !IsUnauthorized(err) {
		t.Errorf("status = %d, IsUnauthorized = %v", apiErr.StatusCode, IsUnauthorized(err))
	}
	if apiErr.Message == "" || apiErr.Fields["ticket"] != "invalid" {
		t.Errorf("fields not parsed: %+v", apiErr)
	}
}

func TestClientGetRetriesOn5xx(t *testing.T) {
	var attempts atomic.Int32
	c := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/version" && attempts.Add(1) >= 3 {
			writeJSON(w, http.StatusOK, `{"data":{"version":"9.2.2","release":"9.2","repoid":"b9984c6d"}}`)
			return
		}
		writeJSON(w, http.StatusBadGateway, `{"data":null,"message":"upstream down"}`)
	}))

	if _, err := c.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion after retries: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestClientPostFailsFast(t *testing.T) {
	var attempts atomic.Int32
	c := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeJSON(w, http.StatusServiceUnavailable, `{"data":null,"message":"maintenance"}`)
	}))

	err := c.do(context.Background(), http.MethodPost, "/cluster/backup", url.Values{"id": {"job1"}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("POST attempts = %d, want 1 (mutations must not retry)", got)
	}
}

func TestClientTicketAuthFlow(t *testing.T) {
	c := testClient(t, Credentials{Username: "root@pam", Password: "hunter2", TOTP: "123456"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			if r.PostForm.Get("username") != "root@pam" || r.PostForm.Get("password") != "hunter2" || r.PostForm.Get("totp") != "123456" {
				t.Errorf("ticket form = %v", r.PostForm)
			}
			// The cap object pushes the body past 512 bytes: the pre-fix
			// hand-rolled ticket read truncated here and failed to parse.
			writeJSON(w, http.StatusOK, `{"data":{"ticket":"TICKET1","CSRFPreventionToken":"CSRF1","cap":{"pad":"`+strings.Repeat("x", 1024)+`"}}}`)
		case "/api2/json/version":
			cookie, err := r.Cookie("PVEAuthCookie")
			if err != nil || cookie.Value != "TICKET1" {
				t.Errorf("PVEAuthCookie = %v (%v)", cookie, err)
			}
			writeJSON(w, http.StatusOK, `{"data":{"version":"9.2.2","release":"9.2","repoid":"b9984c6d"}}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"data":null}`)
		}
	}))

	if _, err := c.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion with ticket auth: %v", err)
	}
}

func TestClientTicketReauthOn401(t *testing.T) {
	var ticketCalls atomic.Int32
	c := testClient(t, Credentials{Username: "root@pam", Password: "hunter2"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			n := ticketCalls.Add(1)
			writeJSON(w, http.StatusOK, `{"data":{"ticket":"TICKET`+string('0'+n)+`","CSRFPreventionToken":"CSRF"}}`)
		case "/api2/json/version":
			cookie, err := r.Cookie("PVEAuthCookie")
			if err != nil || cookie.Value != "TICKET2" {
				// First ticket expired: reject once, forcing re-auth.
				writeJSON(w, http.StatusUnauthorized, `{"data":null,"message":"authentication failed"}`)
				return
			}
			writeJSON(w, http.StatusOK, `{"data":{"version":"9.2.2","release":"9.2","repoid":"b9984c6d"}}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"data":null}`)
		}
	}))

	if _, err := c.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion after ticket re-auth: %v", err)
	}
	if ticketCalls.Load() != 2 {
		t.Errorf("ticket requests = %d, want 2", ticketCalls.Load())
	}
}

// TestEnsureTicketUnauthorizedIsTyped pins the credential-rejection path:
// a 401 from /access/ticket must surface as a typed *APIError (so
// IsUnauthorized classifies it), not an opaque wrapped error.
func TestEnsureTicketUnauthorizedIsTyped(t *testing.T) {
	c := testClient(t, Credentials{Username: "root@pam", Password: "wrong"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/access/ticket" {
			writeJSON(w, http.StatusUnauthorized, `{"errors":{},"message":"bad"}`)
			return
		}
		writeJSON(w, http.StatusNotFound, `{"data":null}`)
	}))

	_, err := c.GetVersion(context.Background())
	if err == nil {
		t.Fatal("GetVersion with rejected credentials: want error, got nil")
	}
	if !IsUnauthorized(err) {
		t.Fatalf("IsUnauthorized(err) = false, want true (err: %v)", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "bad" {
		t.Fatalf("want *APIError with message %q, got %v", "bad", err)
	}
}

func TestClientUnexpectedBody(t *testing.T) {
	c := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>proxy error</html>"))
	}))

	_, err := c.GetVersion(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unexpected response body") {
		t.Fatalf("want envelope error, got %v", err)
	}
}

// TestClientReauthReplaysBody pins the retry path for mutations: after a
// 401 forces re-authentication, the retried POST must carry the same form
// body (a single strings.Reader consumed by the first attempt used to be
// replayed empty).
func TestClientReauthReplaysBody(t *testing.T) {
	c := testClient(t, Credentials{Username: "root@pam", Password: "hunter2"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			writeJSON(w, http.StatusOK, `{"data":{"ticket":"TICKET1","CSRFPreventionToken":"CSRF"}}`)
		case "/api2/json/things":
			if _, err := r.Cookie("PVEAuthCookie"); err != nil {
				writeJSON(w, http.StatusUnauthorized, `{"data":null,"message":"authentication failed"}`)
				return
			}
			if err := r.ParseForm(); err != nil || r.PostForm.Get("name") != "alpha" {
				msg := "retried POST lost its body"
				if err != nil {
					msg = err.Error()
				}
				writeJSON(w, http.StatusBadRequest, `{"message":"`+msg+`"}`)
				return
			}
			writeJSON(w, http.StatusOK, `{"data":"ok"}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"data":null}`)
		}
	}))

	if _, err := c.DoRequest(context.Background(), http.MethodPost, "/things", url.Values{"name": []string{"alpha"}}); err != nil {
		t.Fatalf("DoRequest after re-auth: %v", err)
	}
}

type fakeRootTickets struct {
	mu     sync.Mutex
	minted []string
	forgot []string
}

func (f *fakeRootTickets) RootTicket(_ context.Context, node string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minted = append(f.minted, node)
	return "T", "C", nil
}

func (f *fakeRootTickets) ForgetRootTicket(node string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgot = append(f.forgot, node)
}

func TestRootTicketClusterConfig(t *testing.T) {
	type seen struct{ method, path, cookie, csrf, auth string }
	var (
		mu       sync.Mutex
		requests []seen
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie := ""
		if c, err := r.Cookie("PVEAuthCookie"); err == nil {
			cookie = c.Value
		}
		mu.Lock()
		requests = append(requests, seen{r.Method, strings.TrimPrefix(r.URL.Path, "/api2/json"), cookie, r.Header.Get("CSRFPreventionToken"), r.Header.Get("Authorization")})
		mu.Unlock()
		if r.URL.Path == "/api2/json/cluster/status" {
			writeJSON(w, http.StatusOK, `{"data":[{"type":"cluster","name":"c"},{"type":"node","name":"pve02","local":0},{"type":"node","name":"pve01","local":1}]}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"data":null}`)
	}))
	t.Cleanup(srv.Close)
	fake := &fakeRootTickets{}
	c, err := NewClient(Config{
		URL:            srv.URL,
		Credentials:    Credentials{APIToken: "root@pam!t=abc"},
		Insecure:       true,
		RequestTimeout: 5 * time.Second,
		RootTickets:    fake,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	for _, call := range []struct{ method, path string }{
		{http.MethodPost, "/cluster/config"},
		{http.MethodPost, "/cluster/config"},
		{http.MethodGet, "/cluster/config/join"},
		{http.MethodPost, "/cluster/config/join"},
		{http.MethodPost, "/nodes/pve02/lxc"},
	} {
		if _, err := c.DoRequest(ctx, call.method, call.path, nil); err != nil {
			t.Fatalf("%s %s: %v", call.method, call.path, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	byCall := map[string][]seen{}
	for _, s := range requests {
		key := s.method + " " + s.path
		byCall[key] = append(byCall[key], s)
	}
	if n := len(byCall["GET /cluster/status"]); n != 1 {
		t.Fatalf("GET /cluster/status fetched %d times, want 1", n)
	}
	if s := byCall["GET /cluster/status"][0]; s.auth != "PVEAPIToken=root@pam!t=abc" || s.cookie != "" {
		t.Errorf("GET /cluster/status must use the token, got %+v", s)
	}
	posts := byCall["POST /cluster/config"]
	if len(posts) != 2 {
		t.Fatalf("POST /cluster/config sent %d times, want 2", len(posts))
	}
	for _, s := range posts {
		if s.cookie != "T" || s.csrf != "C" || s.auth != "" {
			t.Errorf("POST /cluster/config must carry the root ticket only, got %+v", s)
		}
	}
	if s := byCall["GET /cluster/config/join"][0]; s.auth == "" || s.cookie != "" {
		t.Errorf("GET /cluster/config/join must use the token, got %+v", s)
	}
	if s := byCall["POST /cluster/config/join"][0]; s.cookie != "T" || s.auth != "" {
		t.Errorf("POST /cluster/config/join must carry the root ticket, got %+v", s)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if want := []string{"pve01", "pve01", "pve01", "pve02"}; !slices.Equal(fake.minted, want) {
		t.Errorf("minted on %v, want %v", fake.minted, want)
	}
	if want := []string{"pve01"}; !slices.Equal(fake.forgot, want) {
		t.Errorf("forgot %v, want %v", fake.forgot, want)
	}
}
