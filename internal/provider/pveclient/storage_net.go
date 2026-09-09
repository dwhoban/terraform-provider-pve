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

// StorageNet mirrors one network-backed storage definition (types nfs,
// cifs, iscsi, iscsidirect) as read from GET /storage/{storage} or written
// by POST /storage and PUT /storage. PVE stores storages in storage.cfg as
// a section config with a `type` discriminator; this struct carries the
// union of the fields the pin defines for these types (every other field
// is optional and absent settings stay nil/"").
//
// Wire quirks normalized here:
//
//   - `content` and `nodes` travel as comma-separated list strings in both
//     directions (pve-storage-content-list / pve-node-list).
//   - Booleans (disable, shared, nowritecache) arrive as 0/1 integers from
//     storage.cfg on most versions and as real booleans on others.
//   - Numeric settings (max-protected-backups) may arrive as JSON numbers
//     or as strings, because the section config stores text on disk.
//
// The pin marks `export`, `share`, `portal`, `target`, and `iscsiprovider`
// as create parameters only — the update verb does not accept them — so
// UpdateStorageNet never sends them and the provider layer forces
// recreation when they change.
type StorageNet struct {
	Storage string `json:"storage,omitempty"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
	Nodes   string `json:"nodes,omitempty"`
	Disable *bool  `json:"disable,omitempty"`
	Shared  *bool  `json:"shared,omitempty"`

	PruneBackups        string `json:"prune-backups,omitempty"`
	MaxProtectedBackups *int64 `json:"max-protected-backups,omitempty"`

	// NFS type.
	Server  string `json:"server,omitempty"`
	Export  string `json:"export,omitempty"`
	Options string `json:"options,omitempty"`

	// CIFS type.
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Domain     string `json:"domain,omitempty"`
	SMBVersion string `json:"smbversion,omitempty"`
	Share      string `json:"share,omitempty"`

	// iSCSI types.
	Portal        string `json:"portal,omitempty"`
	Target        string `json:"target,omitempty"`
	ISCSIProvider string `json:"iscsiprovider,omitempty"`
	NoWriteCache  *bool  `json:"nowritecache,omitempty"`

	// Digest is the read-only storage.cfg revision.
	Digest string `json:"digest,omitempty"`
}

// storageNetRaw mirrors the wire shape with the lenient boolean and
// integer fields left as raw JSON.
type storageNetRaw struct {
	Storage             string          `json:"storage"`
	Type                string          `json:"type"`
	Content             string          `json:"content"`
	Nodes               string          `json:"nodes"`
	Disable             json.RawMessage `json:"disable"`
	Shared              json.RawMessage `json:"shared"`
	PruneBackups        string          `json:"prune-backups"`
	MaxProtectedBackups json.RawMessage `json:"max-protected-backups"`
	Server              string          `json:"server"`
	Export              string          `json:"export"`
	Options             string          `json:"options"`
	Username            string          `json:"username"`
	Password            string          `json:"password"`
	Domain              string          `json:"domain"`
	SMBVersion          string          `json:"smbversion"`
	Share               string          `json:"share"`
	Portal              string          `json:"portal"`
	Target              string          `json:"target"`
	ISCSIProvider       string          `json:"iscsiprovider"`
	NoWriteCache        json.RawMessage `json:"nowritecache"`
	Digest              string          `json:"digest"`
}

// UnmarshalJSON tolerates the 0/1 encoding of the boolean fields and the
// string encoding of numeric fields (see StorageNet), keeping absent
// settings nil.
func (s *StorageNet) UnmarshalJSON(data []byte) error {
	var raw storageNetRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = StorageNet{
		Storage:             raw.Storage,
		Type:                raw.Type,
		Content:             raw.Content,
		Nodes:               raw.Nodes,
		Disable:             decodeBoolishPtr(raw.Disable),
		Shared:              decodeBoolishPtr(raw.Shared),
		PruneBackups:        raw.PruneBackups,
		MaxProtectedBackups: metricsServerInt64FromRaw(raw.MaxProtectedBackups),
		Server:              raw.Server,
		Export:              raw.Export,
		Options:             raw.Options,
		Username:            raw.Username,
		Password:            raw.Password,
		Domain:              raw.Domain,
		SMBVersion:          raw.SMBVersion,
		Share:               raw.Share,
		Portal:              raw.Portal,
		Target:              raw.Target,
		ISCSIProvider:       raw.ISCSIProvider,
		NoWriteCache:        decodeBoolishPtr(raw.NoWriteCache),
		Digest:              raw.Digest,
	}
	return nil
}

// storageNetTypes is the fixed set of network-backed storage types this
// client surface covers; create rejects any other type before hitting the
// wire.
var storageNetTypes = map[string]bool{"nfs": true, "cifs": true, "iscsi": true, "iscsidirect": true}

// storageNetRequired reports the defining fields the pin requires for a
// storage of the given type, as "wire field:Go field" pairs checked in
// order.
func storageNetRequiredChecks(s StorageNet) []error {
	switch s.Type {
	case "nfs":
		return storageNetRequiredStrings(s.Storage, s.Type, [][2]string{
			{"server", s.Server}, {"export", s.Export},
		})
	case "cifs":
		return storageNetRequiredStrings(s.Storage, s.Type, [][2]string{
			{"server", s.Server}, {"share", s.Share},
		})
	case "iscsi", "iscsidirect":
		return storageNetRequiredStrings(s.Storage, s.Type, [][2]string{
			{"portal", s.Portal}, {"target", s.Target},
		})
	}
	return nil
}

// storageNetRequiredStrings turns empty required values into errors.
func storageNetRequiredStrings(storage, typ string, fields [][2]string) []error {
	var errs []error
	for _, f := range fields {
		if f[1] == "" {
			errs = append(errs, fmt.Errorf("pveclient: create storage %s: %s is required for type %s", storage, f[0], typ))
		}
	}
	return errs
}

// CreateStorageNet POSTs /storage with the supplied definition. Storage and
// Type are required per the pin; the type must be one of the network-backed
// types this surface covers (nfs, cifs, iscsi, iscsidirect), and the
// type's defining fields must be present.
func (c *Client) CreateStorageNet(ctx context.Context, s StorageNet) error {
	if s.Storage == "" {
		return fmt.Errorf("pveclient: create storage: storage is required")
	}
	if s.Type == "" {
		return fmt.Errorf("pveclient: create storage %s: type is required", s.Storage)
	}
	if !storageNetTypes[s.Type] {
		return fmt.Errorf("pveclient: create storage %s: unsupported type %q", s.Storage, s.Type)
	}
	if errs := storageNetRequiredChecks(s); len(errs) > 0 {
		return errs[0]
	}
	if err := c.Do(ctx, "POST", "/storage", s, nil); err != nil {
		return fmt.Errorf("pveclient: create storage %s: %w", s.Storage, err)
	}
	return nil
}

// GetStorageNet reads /storage/{storage}. The storage ID is forced to the
// read value; the type is returned as read so callers can type-check the
// response against the component's fixed type.
func (c *Client) GetStorageNet(ctx context.Context, storage string) (*StorageNet, error) {
	var s StorageNet
	if err := c.Do(ctx, "GET", "/storage/"+storage, nil, &s); err != nil {
		return nil, fmt.Errorf("pveclient: read storage %s: %w", storage, err)
	}
	s.Storage = storage
	return &s, nil
}

// UpdateStorageNet PUTs /storage with the supplied fields and translates
// deleteFields into the PVE `delete` query parameter (comma-separated
// field names to clear). The pin's update verb carries no `type` parameter
// and none of the create-only location fields (export, share, portal,
// target, iscsiprovider), so those are never sent; `storage` is a required
// PUT parameter and is forced to the path value.
func (c *Client) UpdateStorageNet(ctx context.Context, storage string, s StorageNet, deleteFields []string) error {
	s.Storage = storage
	s.Type = ""
	s.Export = ""
	s.Share = ""
	s.Portal = ""
	s.Target = ""
	s.ISCSIProvider = ""
	path := "/storage"
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	if err := c.Do(ctx, "PUT", path, s, nil); err != nil {
		return fmt.Errorf("pveclient: update storage %s: %w", storage, err)
	}
	return nil
}

// DeleteStorageNet deletes a storage configuration. The mutation is
// synchronous per the pin (DELETE returns null, no task is spawned).
func (c *Client) DeleteStorageNet(ctx context.Context, storage string) error {
	if err := c.Do(ctx, "DELETE", "/storage/"+storage, nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete storage %s: %w", storage, err)
	}
	return nil
}
