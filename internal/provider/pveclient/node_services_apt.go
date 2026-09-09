// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"strings"
)

// NodeService mirrors one entry of GET /nodes/{node}/services. The three
// state fields carry systemd unit states as reported by PVE (SubState,
// ActiveState, and UnitFileState respectively).
type NodeService struct {
	Name        string `json:"name,omitempty"`
	Service     string `json:"service,omitempty"`
	Description string `json:"desc,omitempty"`
	State       string `json:"state,omitempty"`
	ActiveState string `json:"active-state,omitempty"`
	UnitState   string `json:"unit-state,omitempty"`
}

// NodeServiceOperations is the closed set of service lifecycle verbs the
// pinned API defines on /nodes/{node}/services/{service}.
var NodeServiceOperations = []string{"start", "stop", "restart", "reload"}

// ListNodeServices enumerates GET /nodes/{node}/services.
func (c *Client) ListNodeServices(ctx context.Context, node string) ([]NodeService, error) {
	var out []NodeService
	path := fmt.Sprintf("/nodes/%s/services", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// NodeServiceOp POSTs /nodes/{node}/services/{service}/{operation} and
// returns the worker task UPID. operation must be one of
// NodeServiceOperations; reload falls back to a restart server-side when
// the unit cannot be reloaded.
func (c *Client) NodeServiceOp(ctx context.Context, node, service, operation string) (string, error) {
	valid := false
	for _, op := range NodeServiceOperations {
		if operation == op {
			valid = true
			break
		}
	}
	if !valid {
		return "", fmt.Errorf("pveclient: NodeServiceOp: unknown operation %q (must be one of %s)", operation, strings.Join(NodeServiceOperations, ", "))
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/services/%s/%s", node, service, operation)
	if err := c.Do(ctx, "POST", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// NodeAptRepositoryOption is one options row of a parsed APT repository
// (pin `Options` items).
type NodeAptRepositoryOption struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// NodeAptRepository mirrors one parsed APT repository entry (pin
// `repositories` items nested under files; field names are capitalized per
// the pin).
type NodeAptRepository struct {
	Comment    string                    `json:"Comment,omitempty"`
	Components []string                  `json:"Components,omitempty"`
	Enabled    bool                      `json:"Enabled"`
	FileType   string                    `json:"FileType"`
	Options    []NodeAptRepositoryOption `json:"Options,omitempty"`
	Suites     []string                  `json:"Suites"`
	Types      []string                  `json:"Types"`
	URIs       []string                  `json:"URIs"`
}

// NodeAptRepositoryFile mirrors one parsed repository file (pin `files`
// items). The per-file byte digest is intentionally not modeled: the
// top-level digest covers change detection.
type NodeAptRepositoryFile struct {
	FileType     string              `json:"file-type,omitempty"`
	Path         string              `json:"path,omitempty"`
	Repositories []NodeAptRepository `json:"repositories,omitempty"`
}

// NodeAptRepositoryInfo is one information/warning row about an APT
// repository (pin `infos` items).
type NodeAptRepositoryInfo struct {
	Index    string `json:"index,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Message  string `json:"message,omitempty"`
	Path     string `json:"path,omitempty"`
	Property string `json:"property,omitempty"`
}

// NodeAptStandardRepository is one standard-repository row (pin
// `standard-repos` items). Status is only present when the repository is
// configured (true = enabled, false = disabled); unconfigured standard
// repositories always appear in the list.
type NodeAptStandardRepository struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Status *bool  `json:"status,omitempty"`
}

// NodeAptRepositoryError is one problematic-repository-file row (pin
// `errors` items).
type NodeAptRepositoryError struct {
	Error string `json:"error"`
	Path  string `json:"path"`
}

// NodeAptRepositories mirrors GET /nodes/{node}/apt/repositories.
type NodeAptRepositories struct {
	Digest               string                      `json:"digest"`
	Errors               []NodeAptRepositoryError    `json:"errors,omitempty"`
	Files                []NodeAptRepositoryFile     `json:"files,omitempty"`
	Infos                []NodeAptRepositoryInfo     `json:"infos,omitempty"`
	StandardRepositories []NodeAptStandardRepository `json:"standard-repos,omitempty"`
}

// GetNodeAptRepositories reads GET /nodes/{node}/apt/repositories.
func (c *Client) GetNodeAptRepositories(ctx context.Context, node string) (*NodeAptRepositories, error) {
	var out NodeAptRepositories
	path := fmt.Sprintf("/nodes/%s/apt/repositories", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NodeAptUpdateInput is the payload for POST /nodes/{node}/apt/update.
type NodeAptUpdateInput struct {
	Notify *bool `json:"notify,omitempty"`
	Quiet  *bool `json:"quiet,omitempty"`
}

// NodeAptUpdate resynchronizes the package index (apt-get update) and
// returns the worker task UPID for the caller to wait on.
func (c *Client) NodeAptUpdate(ctx context.Context, node string, in NodeAptUpdateInput) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/apt/update", node)
	if err := c.Do(ctx, "POST", path, in, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// ChangeAptRepositoryInput is the payload for POST
// /nodes/{node}/apt/repositories (pin change_repository). The pin currently
// only supports toggling `enabled` on the repository at `index` within the
// file at `path`; digest optionally guards against concurrent modifications.
type ChangeAptRepositoryInput struct {
	Index   int64  `json:"index"`
	Path    string `json:"path"`
	Enabled *bool  `json:"enabled,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// ChangeAptRepository POSTs the repository change form. The pin returns
// null, so there is no result to decode.
func (c *Client) ChangeAptRepository(ctx context.Context, node string, in ChangeAptRepositoryInput) error {
	path := fmt.Sprintf("/nodes/%s/apt/repositories", node)
	return c.Do(ctx, "POST", path, in, nil)
}

// AddAptStandardRepository PUTs /nodes/{node}/apt/repositories to add the
// standard repository identified by handle (pin add_repository — the pin
// defines add as PUT, unlike the change form's POST). The pin returns null.
func (c *Client) AddAptStandardRepository(ctx context.Context, node, handle string) error {
	path := fmt.Sprintf("/nodes/%s/apt/repositories", node)
	body := struct {
		Handle string `json:"handle"`
	}{Handle: handle}
	return c.Do(ctx, "PUT", path, body, nil)
}
