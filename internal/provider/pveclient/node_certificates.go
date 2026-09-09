// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
)

// UploadNodeCustomCertificateInput is the payload for POST
// /nodes/{node}/certificates/custom. Certificates carries the PEM encoded
// certificate (chain); Key the optional PEM encoded private key. Force
// overwrites existing custom or ACME certificate files; Restart restarts
// pveproxy so the new chain is served immediately.
type UploadNodeCustomCertificateInput struct {
	Certificates string
	Key          string
	Force        bool
	Restart      bool
}

// UploadNodeCustomCertificate POSTs /nodes/{node}/certificates/custom and
// returns the uploaded certificate's info row. The endpoint is synchronous
// per the pin: it returns the certificate object, not a task UPID, so no
// WaitForTask round-trip is needed.
func (c *Client) UploadNodeCustomCertificate(ctx context.Context, node string, in UploadNodeCustomCertificateInput) (*NodeCertificateInfo, error) {
	body := map[string]any{
		"certificates": in.Certificates,
	}
	if in.Key != "" {
		body["key"] = in.Key
	}
	if in.Force {
		body["force"] = true
	}
	if in.Restart {
		body["restart"] = true
	}
	var out NodeCertificateInfo
	path := fmt.Sprintf("/nodes/%s/certificates/custom", node)
	if err := c.Do(ctx, "POST", path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteNodeCustomCertificate DELETEs /nodes/{node}/certificates/custom.
// The pin defines the endpoint as synchronous (returns null), so only the
// error is surfaced. Restart additionally restarts pveproxy and travels as
// the 0/1 integer query parameter PVE expects.
func (c *Client) DeleteNodeCustomCertificate(ctx context.Context, node string, restart bool) error {
	path := fmt.Sprintf("/nodes/%s/certificates/custom", node)
	if restart {
		q := url.Values{}
		q.Set("restart", encodeBoolFlag(true))
		path += "?" + q.Encode()
	}
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// OrderAcmeCertificate POSTs /nodes/{node}/certificates/acme/certificate
// and returns the resulting task UPID. Force overwrites an existing custom
// certificate. Per the pin the call carries no domains: the ACME domains
// come from the node configuration (`acme` entry on PUT
// /nodes/{node}/config), which callers must arrange beforehand.
func (c *Client) OrderAcmeCertificate(ctx context.Context, node string, force bool) (string, error) {
	var body map[string]any
	if force {
		body = map[string]any{"force": true}
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/certificates/acme/certificate", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// RenewAcmeCertificate PUTs /nodes/{node}/certificates/acme/certificate and
// returns the resulting task UPID. Force renews even if expiry is more than
// 30 days away.
func (c *Client) RenewAcmeCertificate(ctx context.Context, node string, force bool) (string, error) {
	var body map[string]any
	if force {
		body = map[string]any{"force": true}
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/certificates/acme/certificate", node)
	if err := c.Do(ctx, "PUT", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// RevokeAcmeCertificate DELETEs /nodes/{node}/certificates/acme/certificate
// and returns the resulting task UPID.
func (c *Client) RevokeAcmeCertificate(ctx context.Context, node string) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/certificates/acme/certificate", node)
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}
