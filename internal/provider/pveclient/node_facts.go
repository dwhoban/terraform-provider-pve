// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
)

// NodeCertificateInfo mirrors one entry of
// GET /nodes/{node}/certificates/info. Every pin field is optional;
// timestamps and key sizes are pointers so absent fields stay nil
// instead of collapsing to 0.
type NodeCertificateInfo struct {
	Filename      string   `json:"filename,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	Issuer        string   `json:"issuer,omitempty"`
	NotAfter      *int64   `json:"notafter,omitempty"`
	NotBefore     *int64   `json:"notbefore,omitempty"`
	PEM           string   `json:"pem,omitempty"`
	PublicKeyBits *int64   `json:"public-key-bits,omitempty"`
	PublicKeyType string   `json:"public-key-type,omitempty"`
	SAN           []string `json:"san,omitempty"`
	Subject       string   `json:"subject,omitempty"`
}

// GetNodeCertificates fetches /nodes/{node}/certificates/info.
func (c *Client) GetNodeCertificates(ctx context.Context, node string) ([]NodeCertificateInfo, error) {
	var out []NodeCertificateInfo
	path := fmt.Sprintf("/nodes/%s/certificates/info", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// NodeVersion mirrors GET /nodes/{node}/version.
type NodeVersion struct {
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
	Version string `json:"version"`
}

// GetNodeVersion fetches /nodes/{node}/version.
func (c *Client) GetNodeVersion(ctx context.Context, node string) (*NodeVersion, error) {
	var out NodeVersion
	path := fmt.Sprintf("/nodes/%s/version", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NodeHosts mirrors GET /nodes/{node}/hosts: the raw /etc/hosts content
// plus the digest used for optimistic concurrency on write.
type NodeHosts struct {
	Data   string `json:"data"`
	Digest string `json:"digest,omitempty"`
}

// GetNodeHosts fetches /nodes/{node}/hosts.
func (c *Client) GetNodeHosts(ctx context.Context, node string) (*NodeHosts, error) {
	var out NodeHosts
	path := fmt.Sprintf("/nodes/%s/hosts", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetNodeHosts POSTs /nodes/{node}/hosts with the whole target file
// content; the optional digest guards against concurrent modification.
func (c *Client) SetNodeHosts(ctx context.Context, node, data, digest string) error {
	body := struct {
		Data   string `json:"data"`
		Digest string `json:"digest,omitempty"`
	}{Data: data, Digest: digest}
	path := fmt.Sprintf("/nodes/%s/hosts", node)
	return c.Do(ctx, "POST", path, body, nil)
}
