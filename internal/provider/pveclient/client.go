// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

// Package pveclient is a thin HTTP client for the Proxmox VE REST API
// (/api2/json). It supports API-token auth (Authorization header) and
// username/password auth (POST /access/ticket -> PVEAuthCookie +
// CSRFPreventionToken), with TLS via insecure-skip or a custom CA pool.
package pveclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	ticketTTL      = 2 * time.Hour
	ticketRefresh  = 5 * time.Minute
	requestTimeout = 60 * time.Second
)

// APIError is returned for non-2xx responses that carry a PVE {"errors":...}
// envelope. It implements error so callers can wrap or format it directly.
type APIError struct {
	StatusCode int
	Errors     []string
	Method     string
	Path       string
}

// Credentials is the subset of fields the PVE HTTP client consumes from
// the credential chain. The credentials package produces a richer
// Credentials struct; pveclient only needs what is required to talk to
// the API.
type Credentials struct {
	Token    string
	Username string
	Password string
	Endpoint string
	Insecure string
	RootCA   string
	OTP      string
}

// Complete reports whether the credentials carry enough material to talk
// to the API: a token on its own, or username and password together.
func (c Credentials) Complete() bool {
	if c.Token != "" {
		return true
	}
	return c.Username != "" && c.Password != ""
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pve api %s %s: HTTP %d: %s",
		e.Method, e.Path, e.StatusCode, strings.Join(e.Errors, "; "))
}

// Client talks to a single PVE endpoint. Construct via NewClient; methods
// are safe for concurrent use.
type Client struct {
	endpoint   string
	httpClient *http.Client
	token      string
	username   string
	password   string
	otp        string

	mu        sync.Mutex
	ticket    string
	csrf      string
	ticketExp time.Time
}

// NewClient builds a Client from the supplied credentials. The TLS config
// honors Insecure (true skips verification) and RootCA (PEM bundle added to
// the trust pool). An error is returned when the credentials are empty
// (defense in depth — the credential chain should have caught this), when
// the endpoint fails to parse, or when RootCA is set but not valid PEM.
func NewClient(creds Credentials) (*Client, error) {
	if !creds.Complete() {
		return nil, errors.New("pveclient: incomplete credentials")
	}
	endpoint := strings.TrimSpace(creds.Endpoint)
	if endpoint == "" {
		return nil, errors.New("pveclient: endpoint is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("pveclient: parse endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("pveclient: endpoint %q must use http or https", endpoint)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if creds.RootCA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(creds.RootCA)) {
			return nil, errors.New("pveclient: root_ca did not contain any valid PEM certificates")
		}
		tlsCfg.RootCAs = pool
	}
	if strings.EqualFold(creds.Insecure, "true") {
		tlsCfg.InsecureSkipVerify = true
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	hc := &http.Client{Transport: transport, Timeout: requestTimeout}
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: hc,
		token:      creds.Token,
		username:   creds.Username,
		password:   creds.Password,
		otp:        creds.OTP,
	}, nil
}

// Endpoint returns the configured endpoint, useful for diagnostics.
func (c *Client) Endpoint() string { return c.endpoint }

// AuthKind returns "token" or "password" depending on how the client is
// configured. Never returns secret material.
func (c *Client) AuthKind() string {
	if c.token != "" {
		return "token"
	}
	return "password"
}

// envelope is the standard PVE response wrapper: {"data": ...} on success,
// sometimes paired with top-level fields. We decode into result only when
// result is non-nil.
type envelope struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors,omitempty"`
}

// Do executes a single API request. path is appended to the endpoint (must
// start with "/"); body is JSON-encoded (nil sends no body); result is
// decoded from the response envelope (nil discards). The CSRF header is
// added automatically for non-GET requests when using password auth.
func (c *Client) Do(ctx context.Context, method, path string, body any, result any) error {
	method = strings.ToUpper(method)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("pveclient: marshal request body for %s %s: %w", method, path, err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return fmt.Errorf("pveclient: build request %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if err := c.sign(req); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pveclient: send %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("pveclient: read response %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode, Method: method, Path: path}
		var env envelope
		if jerr := json.Unmarshal(raw, &env); jerr == nil && len(env.Errors) > 0 {
			// PVE can return errors as a string or as a JSON-encoded string.
			var s string
			if err := json.Unmarshal(env.Errors, &s); err == nil {
				apiErr.Errors = []string{s}
			} else {
				_ = json.Unmarshal(env.Errors, &apiErr.Errors)
			}
		}
		if len(apiErr.Errors) == 0 {
			apiErr.Errors = []string{strings.TrimSpace(string(raw))}
		}
		return apiErr
	}
	if result == nil {
		return nil
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("pveclient: decode envelope %s %s: %w", method, path, err)
	}
	if len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, result); err != nil {
		return fmt.Errorf("pveclient: decode data %s %s: %w", method, path, err)
	}
	return nil
}
