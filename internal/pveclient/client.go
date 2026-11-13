// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Environment variables consulted by the provider configuration. They are
// defined here so the client package documents the full resolution contract.
const (
	EnvAPIToken    = "PVE_API_TOKEN"
	EnvAPITokenID  = "PVE_API_TOKEN_ID"
	EnvAPISecret   = "PVE_API_SECRET"
	EnvUser        = "PVE_USER"
	EnvPassword    = "PVE_PASSWORD"
	EnvTOTP        = "PVE_TOTP"
	EnvAPIURL      = "PVE_API_URL"
	EnvInsecure    = "PVE_INSECURE"
	EnvCACert      = "PVE_CA_CERT"
	EnvTimeout     = "PVE_REQUEST_TIMEOUT"
	EnvTaskTimeout = "PVE_TASK_TIMEOUT"
	DefaultPort    = "8006"
	apiPathPrefix  = "/api2/json"
)

const (
	defaultRequestTimeout = 30 * time.Second
	defaultRetryMax       = 3
	defaultRetryWait      = 250 * time.Millisecond
	maxResponseSnippet    = 512
)

// DefaultTaskTimeout bounds a single UPID task wait unless configured
// otherwise (provider attribute task_timeout / PVE_TASK_TIMEOUT).
const DefaultTaskTimeout = 30 * time.Minute

// Config constructs a Client.
type Config struct {
	// URL is the Proxmox API endpoint, e.g. https://pve.example.com:8006.
	// https is required; port defaults to 8006.
	URL string

	Credentials Credentials

	// Insecure skips TLS certificate verification.
	Insecure bool

	// CACertPEM is a PEM-encoded CA bundle used instead of the system pool.
	CACertPEM string

	// RequestTimeout bounds a single HTTP request. Defaults to 30s.
	RequestTimeout time.Duration

	// RetryMax is the number of retries for idempotent GET requests on
	// connection errors or 5xx. Defaults to 3.
	RetryMax int

	// RetryBaseWait is the base for exponential backoff between retries.
	RetryBaseWait time.Duration

	// TaskTimeout bounds each UPID task wait performed through this
	// client. Defaults to DefaultTaskTimeout (30 minutes).
	TaskTimeout time.Duration

	// Shell runs commands on a node over SSH for the few operations the
	// API cannot perform. Optional; nil means SSH is not configured.
	Shell Shell

	// RootTickets, when set, supplies a root@pam session ticket for calls
	// under /nodes/{node}/ so an API token can perform operations PVE
	// restricts to root@pam. Optional.
	RootTickets RootTicketSource
}

// ShellOutput is the captured output of a remote command.
type ShellOutput struct {
	Stdout string
	Stderr string
}

// Shell executes a command (argv, never a shell string) on a node.
type Shell interface {
	Run(ctx context.Context, node string, argv []string, stdin string) (ShellOutput, error)
}

// RootTicketSource mints a root@pam ticket and CSRF token on a node.
type RootTicketSource interface {
	RootTicket(ctx context.Context, node string) (ticket, csrf string, err error)
}

// APIError is a non-2xx response from the PVE API.
type APIError struct {
	StatusCode int
	Status     string
	Message    string
	Fields     map[string]string
}

func (e *APIError) Error() string {
	if len(e.Fields) > 0 {
		var parts []string
		for k, v := range e.Fields {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
		return fmt.Sprintf("pve api: %s: %s (%s)", e.Status, e.Message, strings.Join(parts, "; "))
	}
	return fmt.Sprintf("pve api: %s: %s", e.Status, e.Message)
}

// IsUnauthorized reports whether the error is an authentication or
// authorization failure (HTTP 401/403).
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}

// Client is a Proxmox VE REST API client for /api2/json.
type Client struct {
	baseURL     *url.URL
	http        *http.Client
	creds       Credentials
	retryMax    int
	retryWait   time.Duration
	taskTimeout time.Duration
	shell       Shell
	rootTickets RootTicketSource

	authMu    sync.Mutex
	ticket    string
	csrfToken string
}

// NewClient validates cfg and returns a ready Client. The returned error
// wraps ErrNoCredentials when the credential set is incomplete.
func NewClient(cfg Config) (*Client, error) {
	if !cfg.Credentials.Complete() {
		return nil, ErrNoCredentials
	}
	u, err := ParseAPIURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.Insecure} //nolint:gosec // opt-in via provider attribute
	if cfg.CACertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.CACertPEM)) {
			return nil, errors.New("ca_cert: no PEM certificates could be parsed")
		}
		tlsCfg.RootCAs = pool
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	retryMax := cfg.RetryMax
	if retryMax < 0 {
		retryMax = defaultRetryMax
	}
	retryWait := cfg.RetryBaseWait
	if retryWait <= 0 {
		retryWait = defaultRetryWait
	}
	taskTimeout := cfg.TaskTimeout
	if taskTimeout <= 0 {
		taskTimeout = DefaultTaskTimeout
	}

	return &Client{
		baseURL:     u,
		http:        &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment}},
		creds:       cfg.Credentials,
		retryMax:    retryMax,
		retryWait:   retryWait,
		taskTimeout: taskTimeout,
		shell:       cfg.Shell,
		rootTickets: cfg.RootTickets,
	}, nil
}

// Shell returns the configured SSH command runner, or nil when the provider
// has no ssh block.
func (c *Client) Shell() Shell { return c.shell }

// rootNode extracts the node of a /nodes/{node}/… API path ("" otherwise).
func rootNode(path string) string {
	rest, ok := strings.CutPrefix(path, "/nodes/")
	if !ok {
		return ""
	}
	node, _, found := strings.Cut(rest, "/")
	if !found {
		return ""
	}
	return node
}

// TaskTimeout returns the per-task wait bound configured on the client.
func (c *Client) TaskTimeout() time.Duration {
	if c.taskTimeout <= 0 {
		return DefaultTaskTimeout
	}
	return c.taskTimeout
}

// ParseAPIURL validates a PVE endpoint: scheme must be https (the PVE API is
// TLS-only), the path must be empty, and the port defaults to 8006.
func ParseAPIURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("api_url is required: set it in the provider block or via %s (e.g. https://pve.example.com:8006)", EnvAPIURL)
	}
	if !strings.Contains(raw, "://") {
		return nil, fmt.Errorf("api_url %q must include the scheme, e.g. https://%s", raw, raw)
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("api_url %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("api_url %q: scheme must be https (the Proxmox VE API is TLS-only)", raw)
	}
	if u.Path != "" {
		return nil, fmt.Errorf("api_url %q: must not contain a path", raw)
	}
	if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), DefaultPort)
	}
	return u, nil
}

// Version is the response of GET /version.
type Version struct {
	Version string `json:"version"`
	Release string `json:"release"`
	Repoid  string `json:"repoid"`
}

// GetVersion returns the PVE version of the endpoint.
func (c *Client) GetVersion(ctx context.Context) (*Version, error) {
	out := &Version{}
	if err := c.do(ctx, http.MethodGet, "/version", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// do performs one API request with authentication and retry policy, then
// decodes the {"data": ...} envelope into out (when non-nil).
//
// Retry policy: only GET requests are retried (on connection errors and
// 500/502/503/504), because retrying a mutation that the server may have
// started processing can duplicate work. All methods are retried on
// connection errors where no response was received only when the request is
// guaranteed not to have been sent, which cannot be distinguished reliably —
// so mutations fail fast.
func (c *Client) do(ctx context.Context, method, path string, params url.Values, out any) error {
	if payload, err := c.doRaw(ctx, method, path, params, out); err != nil {
		return err
	} else if out != nil && payload != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
	}
	return nil
}

// doRaw is do, but returns the raw "data" payload alongside decoding into
// out (when non-nil). The payload is nil for a null data field.
func (c *Client) doRaw(ctx context.Context, method, path string, params url.Values, out any) (json.RawMessage, error) {
	return c.doRequest(ctx, method, path, params, out, true)
}

// doRequest is the one transport path: it builds, authorizes (unless
// authenticate is false), sends, and decodes one API request, applying the
// retry policy. authenticate is false only for the ticket request itself,
// which must skip authorize (it would recurse into ensureTicket) and is
// attempted exactly once.
//
// Retry policy: only GET requests are retried (on connection errors and
// 500/502/503/504), because retrying a mutation that the server may have
// started processing can duplicate work. All methods are retried on
// connection errors where no response was received only when the request is
// guaranteed not to have been sent, which cannot be distinguished reliably —
// so mutations fail fast.
func (c *Client) doRequest(ctx context.Context, method, path string, params url.Values, out any, authenticate bool) (json.RawMessage, error) {
	// The encoded form is re-created per attempt: a retried request (401
	// re-auth) must replay the same body, and a strings.Reader can be
	// consumed only once.
	encoded := ""
	if method != http.MethodGet && method != http.MethodDelete && len(params) > 0 {
		encoded = params.Encode()
	} else if len(params) > 0 {
		path = path + "?" + params.Encode()
	}

	var lastErr error
	reauthed := false
	maxAttempts := 1
	if authenticate {
		if method == http.MethodGet {
			maxAttempts += c.retryMax
		}
		if !c.creds.UsesToken() {
			// One spare attempt to re-authenticate an expired session ticket.
			maxAttempts++
		}
	}
	for attempt := range maxAttempts {
		if attempt > 0 {
			if err := sleepBackoff(ctx, c.retryWait, attempt); err != nil {
				return nil, err
			}
		}
		var body io.Reader
		if encoded != "" {
			body = strings.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL.String()+apiPathPrefix+path, body)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if authenticate {
			if err := c.authorize(ctx, req, rootNode(strings.SplitN(path, "?", 2)[0])); err != nil {
				return nil, err
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s %s: %w", method, path, err)
			// Transport errors on idempotent GET are safe to retry — the
			// request may not have been delivered. Mutations fail fast:
			// the server may have started processing them.
			if method == http.MethodGet && ctx.Err() == nil && attempt+1 < maxAttempts {
				continue
			}
			return nil, lastErr
		}

		apiErr := c.handleResponse(resp, authenticate)
		if apiErr != nil {
			lastErr = apiErr
			var ae *APIError
			if errors.As(apiErr, &ae) {
				// An expired session ticket yields 401: re-authenticate
				// once (handleResponse already dropped the stale ticket).
				// Unauthenticated requests never get here: a rejected
				// password is final on the first and only attempt.
				if authenticate && ae.StatusCode == http.StatusUnauthorized && !c.creds.UsesToken() && !reauthed {
					reauthed = true
					continue
				}
				if isRetryableStatus(ae.StatusCode) && method == http.MethodGet {
					continue
				}
			}
			return nil, apiErr
		}

		payload, err := decodeEnvelope(resp.Body)
		closeErr := resp.Body.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		if out != nil && payload != nil {
			if err := json.Unmarshal(payload, out); err != nil {
				return nil, fmt.Errorf("%s %s: decoding response: %w", method, path, err)
			}
		}
		return payload, nil
	}
	return nil, fmt.Errorf("%s %s: giving up after %d attempts: %w", method, path, maxAttempts, lastErr)
}

// handleResponse converts non-2xx responses into *APIError, consuming and
// closing the body. On success it returns nil and leaves the body open for
// the caller to decode. invalidateTicket is false for the ticket request
// itself: its 401 must neither drop a ticket a sibling call may be storing
// nor take authMu (the ticket fetch holds it).
func (c *Client) handleResponse(resp *http.Response, invalidateTicket bool) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSnippet))
	var errBody struct {
		Message string            `json:"message"`
		Errors  map[string]string `json:"errors"`
	}
	_ = json.Unmarshal(raw, &errBody)

	msg := errBody.Message
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
	}

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Message:    msg,
		Fields:     errBody.Errors,
	}

	// An expired session ticket surfaces as 401 on any method: invalidate
	// and let do()'s caller see the error; ticket re-auth for non-GET
	// happens on the next call because ensureTicket runs per request.
	if invalidateTicket && resp.StatusCode == http.StatusUnauthorized && !c.creds.UsesToken() {
		c.authMu.Lock()
		c.ticket = ""
		c.authMu.Unlock()
	}

	return apiErr
}

// maxResponseBody caps one response body: a config read is a few KiB, so
// 64 MiB is generous while still bounding what a hostile or broken endpoint
// can push into provider memory.
const maxResponseBody = 64 << 20

func decodeEnvelope(r io.Reader) (json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBody {
		return nil, fmt.Errorf("response exceeds %d MiB", maxResponseBody>>20)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("unexpected response body (not a /api2/json envelope): %.200s", strings.TrimSpace(string(raw)))
	}
	return envelope.Data, nil
}

// authorize attaches credentials: the token header for token auth, or the
// session ticket cookie (+ CSRF header for mutating methods).
func (c *Client) authorize(ctx context.Context, req *http.Request, node string) error {
	if c.rootTickets != nil && node != "" {
		ticket, csrf, err := c.rootTickets.RootTicket(ctx, node)
		if err != nil {
			return fmt.Errorf("minting root@pam ticket on node %q over ssh: %w", node, err)
		}
		req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			req.Header.Set("CSRFPreventionToken", csrf)
		}
		return nil
	}
	if c.creds.UsesToken() {
		req.Header.Set("Authorization", "PVEAPIToken="+c.creds.APIToken)
		return nil
	}
	if err := c.ensureTicket(ctx); err != nil {
		return err
	}
	ticket, csrf := c.session()
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: ticket})
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		req.Header.Set("CSRFPreventionToken", csrf)
	}
	return nil
}

// ensureTicket obtains a session ticket via POST /access/ticket when ticket
// credentials are configured, single-flight: authMu is held across the
// fetch, so concurrent callers share one ticket request (a one-time TOTP
// must not be consumed by parallel logins).
func (c *Client) ensureTicket(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.ticket != "" {
		return nil
	}

	form := url.Values{}
	form.Set("username", c.creds.Username)
	form.Set("password", c.creds.Password)
	if c.creds.TOTP != "" {
		form.Set("totp", c.creds.TOTP)
	}
	// authenticate=false routes the request through the one transport path
	// while skipping authorize (which would recurse into ensureTicket) and
	// any retry or re-auth spare: a rejected password is final, and
	// handleResponse reports it as a typed *APIError (without touching
	// authMu) so callers can classify it via IsUnauthorized.
	var envelope struct {
		Ticket              string `json:"ticket"`
		CSRFPreventionToken string `json:"CSRFPreventionToken"`
	}
	payload, err := c.doRequest(ctx, http.MethodPost, "/access/ticket", form, &envelope, false)
	if err != nil {
		return err
	}
	if envelope.Ticket == "" {
		return fmt.Errorf("requesting session ticket: unexpected response: %.200s", strings.TrimSpace(string(payload)))
	}
	c.ticket = envelope.Ticket
	c.csrfToken = envelope.CSRFPreventionToken
	return nil
}

// session returns the current ticket and CSRF token under authMu.
func (c *Client) session() (ticket, csrf string) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.ticket, c.csrfToken
}

func isRetryableStatus(code int) bool {
	return code == 500 || code == 502 || code == 503 || code == 504
}

func sleepBackoff(ctx context.Context, base time.Duration, attempt int) error {
	wait := base << attempt // 250ms, 500ms, 1s, ...
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
