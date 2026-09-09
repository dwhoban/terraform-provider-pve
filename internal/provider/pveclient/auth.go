// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ticketResponse mirrors the body returned by POST /access/ticket.
type ticketResponse struct {
	Data struct {
		Ticket              string `json:"ticket"`
		Username            string `json:"username"`
		CSRFPreventionToken string `json:"CSRFPreventionToken"`
	} `json:"data"`
}

// sign attaches the appropriate authentication to req. Token auth adds the
// Authorization header; password auth lazily fetches a ticket on the first
// call and on expiry, and adds the PVEAuthCookie + CSRFPreventionToken
// headers on non-GET requests.
func (c *Client) sign(req *http.Request) error {
	if c.token != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.token)
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ticket == "" || time.Until(c.ticketExp) < ticketRefresh {
		if err := c.refreshTicket(req.Context()); err != nil {
			return err
		}
	}
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: c.ticket})
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		req.Header.Set("CSRFPreventionToken", c.csrf)
	}
	return nil
}

// refreshTicket POSTs to /access/ticket and caches the returned ticket and
// CSRF token. Must be called with c.mu held.
func (c *Client) refreshTicket(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)
	if c.otp != "" {
		form.Set("otp", c.otp)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/access/ticket", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("pveclient: build /access/ticket request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pveclient: send /access/ticket: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Method: http.MethodPost, Path: "/access/ticket", Errors: []string{fmt.Sprintf("ticket request returned HTTP %d", resp.StatusCode)}}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("pveclient: read /access/ticket response: %w", err)
	}
	var body ticketResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("pveclient: decode /access/ticket response: %w", err)
	}
	if body.Data.Ticket == "" {
		return fmt.Errorf("pveclient: /access/ticket returned empty ticket")
	}
	c.ticket = body.Data.Ticket
	c.csrf = body.Data.CSRFPreventionToken
	c.ticketExp = time.Now().Add(ticketTTL)
	return nil
}
