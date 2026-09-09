// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// NodeStorage is one entry of GET /nodes/{node}/storage (storage index with
// runtime status). The content list decodes from either of PVE's two
// encodings: a comma-separated string, or a JSON array of strings. Flags are
// pointers so callers can distinguish "unset" from "false"; PVE emits them
// as 0/1 integers or booleans depending on version.
type NodeStorage struct {
	Storage      string   `json:"storage"`
	Type         string   `json:"type"`
	Content      []string `json:"content"`
	Shared       *bool    `json:"shared,omitempty"`
	Enabled      *bool    `json:"enabled,omitempty"`
	Active       *bool    `json:"active,omitempty"`
	Used         *int64   `json:"used,omitempty"`
	Total        *int64   `json:"total,omitempty"`
	Avail        *int64   `json:"avail,omitempty"`
	UsedFraction *float64 `json:"used_fraction,omitempty"`
}

// nodeStorageRaw is the wire shape with the lenient fields as raw JSON.
type nodeStorageRaw struct {
	Storage      string          `json:"storage"`
	Type         string          `json:"type"`
	Content      json.RawMessage `json:"content"`
	Shared       json.RawMessage `json:"shared"`
	Enabled      json.RawMessage `json:"enabled"`
	Active       json.RawMessage `json:"active"`
	Used         *int64          `json:"used"`
	Total        *int64          `json:"total"`
	Avail        *int64          `json:"avail"`
	UsedFraction *float64        `json:"used_fraction"`
}

// UnmarshalJSON decodes a storage index row, splitting the content list and
// leniently decoding the status flags.
func (s *NodeStorage) UnmarshalJSON(data []byte) error {
	var raw nodeStorageRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = NodeStorage{
		Storage:      raw.Storage,
		Type:         raw.Type,
		Content:      storageDecodeContentList(raw.Content),
		Shared:       decodeBoolishPtr(raw.Shared),
		Enabled:      decodeBoolishPtr(raw.Enabled),
		Active:       decodeBoolishPtr(raw.Active),
		Used:         raw.Used,
		Total:        raw.Total,
		Avail:        raw.Avail,
		UsedFraction: raw.UsedFraction,
	}
	return nil
}

// storageDecodeContentList decodes PVE's storage content list, which is a
// comma-separated string on the index endpoint but arrives as a JSON array
// on some versions. Absent or null decodes to nil.
func storageDecodeContentList(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return storageSplitList(asString)
	}
	var asArray []string
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray
	}
	return nil
}

// storageSplitList splits a comma-separated PVE list into entries, dropping
// empty segments.
func storageSplitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ListNodeStorages enumerates GET /nodes/{node}/storage. The content filter
// asks PVE to return only stores supporting that content type; the empty
// string omits the query parameter.
func (c *Client) ListNodeStorages(ctx context.Context, node, content string) ([]NodeStorage, error) {
	path := fmt.Sprintf("/nodes/%s/storage", node)
	if content != "" {
		query := url.Values{}
		query.Set("content", content)
		path += "?" + query.Encode()
	}
	var out []NodeStorage
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("pveclient: list node storages %s: %w", path, err)
	}
	return out, nil
}

// NodeStorageContentVerification is the nested verification object PVE
// reports for PBS backups.
type NodeStorageContentVerification struct {
	State *string `json:"state,omitempty"`
	Upid  *string `json:"upid,omitempty"`
}

// NodeStorageContentFile is one entry of GET
// /nodes/{node}/storage/{storage}/content. Pointers mark the optional
// fields so consumers can distinguish "unset" from zero.
type NodeStorageContentFile struct {
	Volid           string                          `json:"volid"`
	Format          string                          `json:"format"`
	Size            *int64                          `json:"size,omitempty"`
	ApproximateSize *int64                          `json:"approximate-size,omitempty"`
	Used            *int64                          `json:"used,omitempty"`
	VMID            *int64                          `json:"vmid,omitempty"`
	Notes           *string                         `json:"notes,omitempty"`
	Ctime           *int64                          `json:"ctime,omitempty"`
	Parent          *string                         `json:"parent,omitempty"`
	Protected       *bool                           `json:"protected,omitempty"`
	Encrypted       *string                         `json:"encrypted,omitempty"`
	Verification    *NodeStorageContentVerification `json:"verification,omitempty"`
}

// nodeStorageContentFileRaw is the wire shape; protected arrives as a
// boolish value (true/false or 0/1 depending on version and plugin).
type nodeStorageContentFileRaw struct {
	Volid           string                          `json:"volid"`
	Format          string                          `json:"format"`
	Size            *int64                          `json:"size"`
	ApproximateSize *int64                          `json:"approximate-size"`
	Used            *int64                          `json:"used"`
	VMID            *int64                          `json:"vmid"`
	Notes           *string                         `json:"notes"`
	Ctime           *int64                          `json:"ctime"`
	Parent          *string                         `json:"parent"`
	Protected       json.RawMessage                 `json:"protected"`
	Encrypted       *string                         `json:"encrypted"`
	Verification    *NodeStorageContentVerification `json:"verification"`
}

// UnmarshalJSON decodes a content row, leniently decoding the protected
// flag.
func (f *NodeStorageContentFile) UnmarshalJSON(data []byte) error {
	var raw nodeStorageContentFileRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = NodeStorageContentFile{
		Volid:           raw.Volid,
		Format:          raw.Format,
		Size:            raw.Size,
		ApproximateSize: raw.ApproximateSize,
		Used:            raw.Used,
		VMID:            raw.VMID,
		Notes:           raw.Notes,
		Ctime:           raw.Ctime,
		Parent:          raw.Parent,
		Protected:       decodeBoolishPtr(raw.Protected),
		Encrypted:       raw.Encrypted,
		Verification:    raw.Verification,
	}
	return nil
}

// ListStorageContent enumerates GET /nodes/{node}/storage/{storage}/content.
// The content filter asks PVE to return only files of that content type; the
// empty string omits the query parameter.
func (c *Client) ListStorageContent(ctx context.Context, node, storage, content string) ([]NodeStorageContentFile, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/content", node, storage)
	if content != "" {
		query := url.Values{}
		query.Set("content", content)
		path += "?" + query.Encode()
	}
	var out []NodeStorageContentFile
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("pveclient: list storage content %s: %w", path, err)
	}
	return out, nil
}
