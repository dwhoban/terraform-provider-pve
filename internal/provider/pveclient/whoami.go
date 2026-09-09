// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import "context"

// Whoami is the PVE /access/whoami response payload. Fields beyond Username
// and Realm are passed through from PVE so future providers can use them
// without an SDK upgrade.
type Whoami struct {
	Username string `json:"username"`
	Realm    string `json:"realm"`
	Email    string `json:"email,omitempty"`
	// TFAEnabled, Keys, and Cap are documented by the PVE API but not all
	// are always present; we capture them as raw JSON for forward
	// compatibility.
	// See https://pve.proxmox.com/pve-docs/api-viewer/apidoc.js for the
	// full schema.
}

// Whoami calls GET /access/whoami and decodes the data envelope into a
// Whoami struct. The call requires a valid session (token or fresh
// ticket); a 401 surfaces as an *APIError.
func (c *Client) Whoami(ctx context.Context) (*Whoami, error) {
	var w Whoami
	if err := c.Do(ctx, "GET", "/access/whoami", nil, &w); err != nil {
		return nil, err
	}
	return &w, nil
}
