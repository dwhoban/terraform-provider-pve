// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PoolBoolPtr returns a pointer to b, for building pool request structs.
func PoolBoolPtr(b bool) *bool { return &b }

// PoolMember is one member of a resource pool, as returned by the pool
// configuration endpoints. VMID is set for guest members (qemu, lxc,
// openvz); Storage is set for storage members.
type PoolMember struct {
	ID      string `json:"id"`
	Node    string `json:"node,omitempty"`
	Storage string `json:"storage,omitempty"`
	Type    string `json:"type,omitempty"`
	VMID    *int64 `json:"vmid,omitempty"`
}

// Pool is a Proxmox VE resource pool (/pools). Comment is absent on pools
// created without one; Members is only populated by the per-pool read.
type Pool struct {
	PoolID  string       `json:"poolid"`
	Comment string       `json:"comment,omitempty"`
	Members []PoolMember `json:"members,omitempty"`
}

// PoolUpdate is the request body of PUT /pools/{poolid}. VMs and Storages
// are the PVE `pve-vmid-list` / `pve-storage-id-list` members to add or
// (when Delete is true) remove. AllowMove allows adding a guest that is
// already in another pool; it is only meaningful on the add form.
type PoolUpdate struct {
	Comment   string
	VMs       []int64
	Storages  []string
	Delete    bool
	AllowMove *bool
}

// poolUpdateWire mirrors the wire shape of PoolUpdate: the hyphenated
// allow-move key and the comma-joined list strings.
type poolUpdateWire struct {
	Comment   string `json:"comment,omitempty"`
	VMs       string `json:"vms,omitempty"`
	Storages  string `json:"storage,omitempty"`
	Delete    *bool  `json:"delete,omitempty"`
	AllowMove *bool  `json:"allow-move,omitempty"`
}

// MarshalJSON emits the wire shape, comma-joining the member lists and
// encoding delete only for the remove form.
func (u PoolUpdate) MarshalJSON() ([]byte, error) {
	w := poolUpdateWire{
		Comment:   u.Comment,
		VMs:       poolJoinVMIDs(u.VMs),
		Storages:  strings.Join(u.Storages, ","),
		AllowMove: u.AllowMove,
	}
	if u.Delete {
		w.Delete = PoolBoolPtr(true)
	}
	return json.Marshal(w)
}

// poolJoinVMIDs renders VMIDs as PVE's comma-separated wire string.
func poolJoinVMIDs(vms []int64) string {
	if len(vms) == 0 {
		return ""
	}
	parts := make([]string, len(vms))
	for i, vmid := range vms {
		parts[i] = fmt.Sprintf("%d", vmid)
	}
	return strings.Join(parts, ",")
}

// ListPools returns the pools from GET /pools. PVE only populates Members
// on the per-pool read, so list entries carry empty member slices.
func (c *Client) ListPools(ctx context.Context) ([]Pool, error) {
	var out []Pool
	if err := c.Do(ctx, "GET", "/pools", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPool reads GET /pools/{poolid}. The pin types the response as an
// array (the handler is shared with the list endpoint), so the first
// element is the pool configuration and an empty array decodes as a nil
// pool (caller treats that as not found).
func (c *Client) GetPool(ctx context.Context, poolid string) (*Pool, error) {
	var out []Pool
	path := fmt.Sprintf("/pools/%s", poolid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &out[0], nil
}

// CreatePool POSTs /pools with the supplied poolid and optional comment.
// The pin's create endpoint does not accept members; attach them with
// UpdatePool after creation.
func (c *Client) CreatePool(ctx context.Context, poolid, comment string) error {
	body := Pool{PoolID: poolid, Comment: comment}
	return c.Do(ctx, "POST", "/pools", body, nil)
}

// UpdatePool PUTs /pools/{poolid} with the supplied update.
func (c *Client) UpdatePool(ctx context.Context, poolid string, update PoolUpdate) error {
	path := fmt.Sprintf("/pools/%s", poolid)
	return c.Do(ctx, "PUT", path, update, nil)
}

// DeletePool DELETEs /pools/{poolid}. PVE refuses to delete non-empty
// pools; callers must remove the members first.
func (c *Client) DeletePool(ctx context.Context, poolid string) error {
	path := fmt.Sprintf("/pools/%s", poolid)
	return c.Do(ctx, "DELETE", path, nil, nil)
}
