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

// StorageRemote mirrors one storage definition as read from
// GET /storage/{storage} or written by POST /storage and PUT /storage.
// PVE stores storages in storage.cfg as a section config with a `type`
// discriminator; this struct carries the union of the fields the pin
// defines for the remote types pbs, cephfs, and rbd (every other field is
// optional and absent settings stay nil/"").
//
// Wire quirks normalized here:
//
//   - `content` travels as a comma-separated list string in both
//     directions (pve-storage-content-list).
//   - Booleans (disable, fuse, krbd, skip-cert-verification) arrive as 0/1
//     integers from storage.cfg on most versions and as real booleans on
//     others.
//   - Numeric settings (port, max-protected-backups) may arrive as JSON
//     numbers or as strings, because the section config stores text on
//     disk.
type StorageRemote struct {
	Storage string `json:"storage,omitempty"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
	Disable *bool  `json:"disable,omitempty"`
	Nodes   string `json:"nodes,omitempty"`

	// Proxmox Backup Server type.
	Server               string `json:"server,omitempty"`
	Port                 *int64 `json:"port,omitempty"`
	Datastore            string `json:"datastore,omitempty"`
	Username             string `json:"username,omitempty"`
	Password             string `json:"password,omitempty"`
	Fingerprint          string `json:"fingerprint,omitempty"`
	Namespace            string `json:"namespace,omitempty"`
	MasterPubkey         string `json:"master-pubkey,omitempty"`
	MaxProtectedBackups  *int64 `json:"max-protected-backups,omitempty"`
	PruneBackups         string `json:"prune-backups,omitempty"`
	SkipCertVerification *bool  `json:"skip-cert-verification,omitempty"`

	// CephFS and RBD share the external-cluster fields.
	Monhost string `json:"monhost,omitempty"`
	Keyring string `json:"keyring,omitempty"`

	// CephFS type.
	FsName string `json:"fs-name,omitempty"`
	Subdir string `json:"subdir,omitempty"`
	Path   string `json:"path,omitempty"`
	Fuse   *bool  `json:"fuse,omitempty"`

	// RBD type.
	Pool          string `json:"pool,omitempty"`
	DataPool      string `json:"data-pool,omitempty"`
	Authsupported string `json:"authsupported,omitempty"`
	KRBD          *bool  `json:"krbd,omitempty"`

	// Digest is the read-only storage.cfg revision.
	Digest string `json:"digest,omitempty"`
}

// storageRemoteRaw mirrors the wire shape with the lenient boolean and
// integer fields left as raw JSON.
type storageRemoteRaw struct {
	Storage              string          `json:"storage"`
	Type                 string          `json:"type"`
	Content              string          `json:"content"`
	Disable              json.RawMessage `json:"disable"`
	Nodes                string          `json:"nodes"`
	Server               string          `json:"server"`
	Port                 json.RawMessage `json:"port"`
	Datastore            string          `json:"datastore"`
	Username             string          `json:"username"`
	Password             string          `json:"password"`
	Fingerprint          string          `json:"fingerprint"`
	Namespace            string          `json:"namespace"`
	MasterPubkey         string          `json:"master-pubkey"`
	MaxProtectedBackups  json.RawMessage `json:"max-protected-backups"`
	PruneBackups         string          `json:"prune-backups"`
	SkipCertVerification json.RawMessage `json:"skip-cert-verification"`
	Monhost              string          `json:"monhost"`
	Keyring              string          `json:"keyring"`
	FsName               string          `json:"fs-name"`
	Subdir               string          `json:"subdir"`
	Path                 string          `json:"path"`
	Fuse                 json.RawMessage `json:"fuse"`
	Pool                 string          `json:"pool"`
	DataPool             string          `json:"data-pool"`
	Authsupported        string          `json:"authsupported"`
	KRBD                 json.RawMessage `json:"krbd"`
	Digest               string          `json:"digest"`
}

// UnmarshalJSON tolerates the 0/1 encoding of the boolean fields and the
// string encoding of numeric fields (see StorageRemote), keeping absent
// settings nil.
func (s *StorageRemote) UnmarshalJSON(data []byte) error {
	var raw storageRemoteRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = StorageRemote{
		Storage:              raw.Storage,
		Type:                 raw.Type,
		Content:              raw.Content,
		Disable:              decodeBoolishPtr(raw.Disable),
		Nodes:                raw.Nodes,
		Server:               raw.Server,
		Port:                 metricsServerInt64FromRaw(raw.Port),
		Datastore:            raw.Datastore,
		Username:             raw.Username,
		Password:             raw.Password,
		Fingerprint:          raw.Fingerprint,
		Namespace:            raw.Namespace,
		MasterPubkey:         raw.MasterPubkey,
		MaxProtectedBackups:  metricsServerInt64FromRaw(raw.MaxProtectedBackups),
		PruneBackups:         raw.PruneBackups,
		SkipCertVerification: decodeBoolishPtr(raw.SkipCertVerification),
		Monhost:              raw.Monhost,
		Keyring:              raw.Keyring,
		FsName:               raw.FsName,
		Subdir:               raw.Subdir,
		Path:                 raw.Path,
		Fuse:                 decodeBoolishPtr(raw.Fuse),
		Pool:                 raw.Pool,
		DataPool:             raw.DataPool,
		Authsupported:        raw.Authsupported,
		KRBD:                 decodeBoolishPtr(raw.KRBD),
		Digest:               raw.Digest,
	}
	return nil
}

// storageRemoteTypes is the fixed set of remote storage types this client
// surface covers; create rejects any other type before hitting the wire.
var storageRemoteTypes = map[string]bool{"pbs": true, "cephfs": true, "rbd": true}

// CreateStorageRemote POSTs /storage with the supplied definition.
// Storage and Type are required per the pin; the type must be one of the
// remote types this surface covers (pbs, cephfs, rbd).
func (c *Client) CreateStorageRemote(ctx context.Context, s StorageRemote) error {
	if s.Storage == "" {
		return fmt.Errorf("pveclient: create storage: storage is required")
	}
	if s.Type == "" {
		return fmt.Errorf("pveclient: create storage %s: type is required", s.Storage)
	}
	if !storageRemoteTypes[s.Type] {
		return fmt.Errorf("pveclient: create storage %s: unsupported type %q", s.Storage, s.Type)
	}
	if err := c.Do(ctx, "POST", "/storage", s, nil); err != nil {
		return fmt.Errorf("pveclient: create storage %s: %w", s.Storage, err)
	}
	return nil
}

// GetStorageRemote reads /storage/{storage}. The storage ID and type are
// forced to the read values so callers can type-check the response.
func (c *Client) GetStorageRemote(ctx context.Context, storage string) (*StorageRemote, error) {
	var s StorageRemote
	if err := c.Do(ctx, "GET", "/storage/"+storage, nil, &s); err != nil {
		return nil, fmt.Errorf("pveclient: read storage %s: %w", storage, err)
	}
	s.Storage = storage
	return &s, nil
}

// UpdateStorageRemote PUTs /storage with the supplied fields and
// translates deleteFields into the PVE `delete` query parameter
// (comma-separated field names to clear). The pin's update verb carries no
// `type` parameter — the storage type is immutable — so Type is never
// sent; `storage` is a required PUT parameter and is forced to the path
// value.
func (c *Client) UpdateStorageRemote(ctx context.Context, storage string, s StorageRemote, deleteFields []string) error {
	s.Storage = storage
	s.Type = ""
	path := "/storage"
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	if err := c.Do(ctx, "PUT", path, s, nil); err != nil {
		return fmt.Errorf("pveclient: update storage %s: %w", storage, err)
	}
	return nil
}

// DeleteStorageRemote deletes a storage configuration. The mutation is
// synchronous per the pin (DELETE returns null, no task is spawned).
func (c *Client) DeleteStorageRemote(ctx context.Context, storage string) error {
	if err := c.Do(ctx, "DELETE", "/storage/"+storage, nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete storage %s: %w", storage, err)
	}
	return nil
}
