// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// StorageLocalConfig is one block/local storage configuration (GET/PUT/
// DELETE /storage/{storage}, POST /storage) for the lvm, lvmthin, zfspool,
// and dir types. The union of fields is shared because the pin defines a
// single parameter set and PVE validates per type upstream. Lists travel
// as comma-separated strings, prune-backups as a property string on
// POST/PUT but as an object or property string on GET, and boolean fields
// tolerate the 0/1 int encoding PVE emits for storage.cfg values.
type StorageLocalConfig struct {
	Storage string
	Type    string
	Digest  string

	Content             []string
	Nodes               []string
	Disable             *bool
	Shared              *bool
	BWLimit             string
	PruneBackups        *BackupPruneBackups
	MaxProtectedBackups *int64

	VGName             string
	Base               string
	SafeRemove         *bool
	SafeRemoveStepSize *int64
	TaggedOnly         *bool

	ThinPool string

	Pool      string
	BlockSize string
	Sparse    *bool

	Path           string
	IsMountpoint   string
	CreateBasePath *bool
	CreateSubdirs  *bool
	Mkdir          *bool
}

// storageLocalWire mirrors the wire shape of StorageLocalConfig with
// hyphenated keys, joined lists, and lenient fields as json.RawMessage.
type storageLocalWire struct {
	Storage             string          `json:"storage,omitempty"`
	Type                string          `json:"type,omitempty"`
	Digest              string          `json:"digest,omitempty"`
	Content             json.RawMessage `json:"content,omitempty"`
	Nodes               json.RawMessage `json:"nodes,omitempty"`
	Disable             json.RawMessage `json:"disable,omitempty"`
	Shared              json.RawMessage `json:"shared,omitempty"`
	BWLimit             string          `json:"bwlimit,omitempty"`
	PruneBackups        json.RawMessage `json:"prune-backups,omitempty"`
	MaxProtectedBackups json.RawMessage `json:"max-protected-backups,omitempty"`

	VGName             string          `json:"vgname,omitempty"`
	Base               string          `json:"base,omitempty"`
	SafeRemove         json.RawMessage `json:"saferemove,omitempty"`
	SafeRemoveStepSize json.RawMessage `json:"saferemove-stepsize,omitempty"`
	TaggedOnly         json.RawMessage `json:"tagged_only,omitempty"`

	ThinPool string `json:"thinpool,omitempty"`

	Pool      string          `json:"pool,omitempty"`
	BlockSize string          `json:"blocksize,omitempty"`
	Sparse    json.RawMessage `json:"sparse,omitempty"`

	Path           string          `json:"path,omitempty"`
	IsMountpoint   string          `json:"is_mountpoint,omitempty"`
	CreateBasePath json.RawMessage `json:"create-base-path,omitempty"`
	CreateSubdirs  json.RawMessage `json:"create-subdirs,omitempty"`
	Mkdir          json.RawMessage `json:"mkdir,omitempty"`
}

// storageLocalBoolPtr returns a pointer to b, for building config structs.
func storageLocalBoolPtr(b bool) *bool { return &b }

// storageLocalInt64Ptr returns a pointer to i, for building config structs.
func storageLocalInt64Ptr(i int64) *int64 { return &i }

// storageLocalListRaw decodes a raw list value that arrives as either a
// comma-separated string or a JSON array.
func storageLocalListRaw(raw json.RawMessage) []string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return haSplitList(s)
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

// storageLocalInt64Raw decodes a raw integer that arrives as a JSON number
// or a string.
func storageLocalInt64Raw(raw json.RawMessage) *int64 {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return &n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return &v
		}
	}
	return nil
}

// storageLocalBoolishPtr decodes a raw boolish value into a *bool, keeping
// nil for absent/null. Storage.cfg values additionally arrive as quoted
// "1"/"0" strings on some PVE versions.
func storageLocalBoolishPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "1", "true", "yes", "on":
				return storageLocalBoolPtr(true)
			case "0", "false", "no", "off":
				return storageLocalBoolPtr(false)
			}
			return nil
		}
	}
	v := decodeBoolish(trimmed)
	return &v
}

// MarshalJSON encodes the lists as PVE's comma-separated wire strings,
// prune-backups as a property string, and the flags as JSON booleans.
func (c StorageLocalConfig) MarshalJSON() ([]byte, error) {
	w := storageLocalWire{
		Storage:             c.Storage,
		Type:                c.Type,
		Digest:              c.Digest,
		BWLimit:             c.BWLimit,
		VGName:              c.VGName,
		Base:                c.Base,
		ThinPool:            c.ThinPool,
		Pool:                c.Pool,
		BlockSize:           c.BlockSize,
		Path:                c.Path,
		IsMountpoint:        c.IsMountpoint,
		Disable:             backupBoolRaw(c.Disable),
		Shared:              backupBoolRaw(c.Shared),
		SafeRemove:          backupBoolRaw(c.SafeRemove),
		TaggedOnly:          backupBoolRaw(c.TaggedOnly),
		Sparse:              backupBoolRaw(c.Sparse),
		CreateBasePath:      backupBoolRaw(c.CreateBasePath),
		CreateSubdirs:       backupBoolRaw(c.CreateSubdirs),
		Mkdir:               backupBoolRaw(c.Mkdir),
		MaxProtectedBackups: haInt64Raw(c.MaxProtectedBackups),
		SafeRemoveStepSize:  haInt64Raw(c.SafeRemoveStepSize),
	}
	if len(c.Content) > 0 {
		w.Content = backupJSONString(strings.Join(c.Content, ","))
	}
	if len(c.Nodes) > 0 {
		w.Nodes = backupJSONString(strings.Join(c.Nodes, ","))
	}
	if c.PruneBackups != nil {
		w.PruneBackups = backupJSONString(backupPruneBackupsString(c.PruneBackups))
	}
	return json.Marshal(w)
}

// UnmarshalJSON splits the wire lists, decodes the boolish flags and
// lenient ints, and decodes prune-backups as an object or property string.
func (c *StorageLocalConfig) UnmarshalJSON(data []byte) error {
	var w storageLocalWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	c.Storage = w.Storage
	c.Type = w.Type
	c.Digest = w.Digest
	c.Content = storageLocalListRaw(w.Content)
	c.Nodes = storageLocalListRaw(w.Nodes)
	c.Disable = storageLocalBoolishPtr(w.Disable)
	c.Shared = storageLocalBoolishPtr(w.Shared)
	c.BWLimit = w.BWLimit
	c.MaxProtectedBackups = storageLocalInt64Raw(w.MaxProtectedBackups)
	c.VGName = w.VGName
	c.Base = w.Base
	c.SafeRemove = storageLocalBoolishPtr(w.SafeRemove)
	c.SafeRemoveStepSize = storageLocalInt64Raw(w.SafeRemoveStepSize)
	c.TaggedOnly = storageLocalBoolishPtr(w.TaggedOnly)
	c.ThinPool = w.ThinPool
	c.Pool = w.Pool
	c.BlockSize = w.BlockSize
	c.Sparse = storageLocalBoolishPtr(w.Sparse)
	c.Path = w.Path
	c.IsMountpoint = w.IsMountpoint
	c.CreateBasePath = storageLocalBoolishPtr(w.CreateBasePath)
	c.CreateSubdirs = storageLocalBoolishPtr(w.CreateSubdirs)
	c.Mkdir = storageLocalBoolishPtr(w.Mkdir)
	prune, err := backupPruneBackupsFromRaw(w.PruneBackups)
	if err != nil {
		return fmt.Errorf("decoding prune-backups: %w", err)
	}
	c.PruneBackups = prune
	return nil
}

// GetStorageLocal reads /storage/{storage}.
func (c *Client) GetStorageLocal(ctx context.Context, storage string) (*StorageLocalConfig, error) {
	var out StorageLocalConfig
	if err := c.Do(ctx, "GET", fmt.Sprintf("/storage/%s", storage), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateStorageLocal POSTs /storage with the supplied configuration. The
// call is synchronous per the pin (returns a summary object, no task).
func (c *Client) CreateStorageLocal(ctx context.Context, cfg StorageLocalConfig) error {
	return c.Do(ctx, "POST", "/storage", cfg, nil)
}

// UpdateStorageLocal PUTs /storage/{storage} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter. The
// call is synchronous per the pin.
func (c *Client) UpdateStorageLocal(ctx context.Context, storage string, cfg StorageLocalConfig, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/storage/%s", storage), deleteFields)
	return c.Do(ctx, "PUT", path, cfg, nil)
}

// DeleteStorageLocal DELETEs /storage/{storage}. The call is synchronous
// per the pin (returns null, no task).
func (c *Client) DeleteStorageLocal(ctx context.Context, storage string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/storage/%s", storage), nil, nil)
}
