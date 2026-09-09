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

// SdnFabricProtocol values fixed by the two fabric resources this provider
// manages. The pin's fabric sections also define `wireguard` and `bgp`,
// which the provider does not model.
const (
	SdnFabricProtocolOSPF       = "ospf"
	SdnFabricProtocolOpenfabric = "openfabric"
)

// SdnLockOptions carries the shared optional parameters of the SDN apply
// (PUT /cluster/sdn) and rollback (POST /cluster/sdn/rollback) verbs.
// ReleaseLock is nil to keep the pin's default (release after success).
type SdnLockOptions struct {
	LockToken   string
	ReleaseLock *bool
}

// sdnLockBody is the JSON body for the lock-token and release-lock
// parameters. Both fields are omitempty: absent options stay absent on the
// wire so PVE applies its defaults.
type sdnLockBody struct {
	LockToken   string `json:"lock-token,omitempty"`
	ReleaseLock *bool  `json:"release-lock,omitempty"`
}

// The return type is `any` on purpose: a typed nil *sdnLockBody would be a
// non-nil interface value and Do would marshal it to the literal `null`.
func (o SdnLockOptions) body() any {
	if o.LockToken == "" && o.ReleaseLock == nil {
		return nil
	}
	return &sdnLockBody{LockToken: o.LockToken, ReleaseLock: o.ReleaseLock}
}

// ApplySdn PUTs /cluster/sdn ("Apply sdn controller changes && reload"),
// pushing the whole pending SDN configuration cluster-wide. The pin types
// the return value as a string: the reload worker's task UPID, which the
// caller should wait on.
func (c *Client) ApplySdn(ctx context.Context, opts SdnLockOptions) (string, error) {
	var upid string
	if err := c.Do(ctx, "PUT", "/cluster/sdn", opts.body(), &upid); err != nil {
		return "", fmt.Errorf("applying SDN configuration: %w", err)
	}
	return upid, nil
}

// RollbackSdn POSTs /cluster/sdn/rollback, discarding all pending SDN
// configuration changes. The pin returns null: the operation is synchronous.
func (c *Client) RollbackSdn(ctx context.Context, opts SdnLockOptions) error {
	if err := c.Do(ctx, "POST", "/cluster/sdn/rollback", opts.body(), nil); err != nil {
		return fmt.Errorf("rolling back SDN configuration: %w", err)
	}
	return nil
}

// SdnFabricRedistribute is one OSPF redistribute entry: the protocol routes
// are sourced from, with an optional route map filter.
type SdnFabricRedistribute struct {
	Source   string `json:"source"`
	RouteMap string `json:"route-map,omitempty"`
}

// UnmarshalJSON accepts both the JSON object form PVE returns on some
// versions and the `source=<proto>,route-map=<id>` property-string form.
func (r *SdnFabricRedistribute) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if strings.HasPrefix(raw, "{") {
		type plain SdnFabricRedistribute
		var p plain
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		*r = SdnFabricRedistribute(p)
		return nil
	}
	fields, err := sdnFabricParsePropString(strings.Trim(raw, `"`))
	if err != nil {
		return err
	}
	r.Source = fields["source"]
	r.RouteMap = fields["route-map"]
	return nil
}

// SdnFabricInterface is one network interface entry of a fabric node. The
// pin defines per-protocol extras: NetworkType is OSPF-only,
// HelloMultiplier is OpenFabric-only (2-100).
type SdnFabricInterface struct {
	Name            string `json:"name"`
	IP              string `json:"ip,omitempty"`
	IP6             string `json:"ip6,omitempty"`
	NetworkType     string `json:"network_type,omitempty"`
	HelloMultiplier *int64 `json:"hello_multiplier,omitempty"`
}

// UnmarshalJSON accepts both the JSON object form and the
// `name=<iface>,ip=<cidr>,...` property-string form PVE emits.
func (i *SdnFabricInterface) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if strings.HasPrefix(raw, "{") {
		type plain SdnFabricInterface
		var p plain
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		*i = SdnFabricInterface(p)
		return nil
	}
	fields, err := sdnFabricParsePropString(strings.Trim(raw, `"`))
	if err != nil {
		return err
	}
	i.Name = fields["name"]
	i.IP = fields["ip"]
	i.IP6 = fields["ip6"]
	i.NetworkType = fields["network_type"]
	if v, ok := fields["hello_multiplier"]; ok && v != "" {
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return fmt.Errorf("parsing hello_multiplier %q: %w", v, err)
		}
		i.HelloMultiplier = &n
	}
	return nil
}

// sdnFabricParsePropString splits a PVE property string (`k=v,k=v`) into a
// map. Keys without a value map to the empty string.
func sdnFabricParsePropString(s string) (map[string]string, error) {
	out := make(map[string]string)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			return nil, fmt.Errorf("parsing property string %q: entry %q has no `=`", s, part)
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out, nil
}

// SdnFabric is one fabric configuration entry
// (/cluster/sdn/fabrics/fabric). Protocol selects the section type; Area
// and Redistribute apply to OSPF, the intervals to OpenFabric, and
// RouteFilter to both. Digest is read-only (returned by GET, accepted by
// PUT for optimistic locking).
type SdnFabric struct {
	ID            string                  `json:"id"`
	Protocol      string                  `json:"protocol,omitempty"`
	IPPrefix      string                  `json:"ip_prefix,omitempty"`
	IP6Prefix     string                  `json:"ip6_prefix,omitempty"`
	Area          string                  `json:"area,omitempty"`
	CsnpInterval  *float64                `json:"csnp_interval,omitempty"`
	HelloInterval *float64                `json:"hello_interval,omitempty"`
	RouteFilter   string                  `json:"route_filter,omitempty"`
	Redistribute  []SdnFabricRedistribute `json:"redistribute,omitempty"`
	Digest        string                  `json:"digest,omitempty"`
}

// ListSdnFabrics enumerates GET /cluster/sdn/fabrics/fabric.
func (c *Client) ListSdnFabrics(ctx context.Context) ([]SdnFabric, error) {
	var out []SdnFabric
	if err := c.Do(ctx, "GET", "/cluster/sdn/fabrics/fabric", nil, &out); err != nil {
		return nil, fmt.Errorf("listing SDN fabrics: %w", err)
	}
	return out, nil
}

// GetSdnFabric reads GET /cluster/sdn/fabrics/fabric/{id}.
func (c *Client) GetSdnFabric(ctx context.Context, id string) (*SdnFabric, error) {
	var out SdnFabric
	if err := c.Do(ctx, "GET", "/cluster/sdn/fabrics/fabric/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, fmt.Errorf("reading SDN fabric %s: %w", id, err)
	}
	return &out, nil
}

// CreateSdnFabric POSTs /cluster/sdn/fabrics/fabric. Protocol must be set
// by the caller.
func (c *Client) CreateSdnFabric(ctx context.Context, fabric SdnFabric) error {
	if err := c.Do(ctx, "POST", "/cluster/sdn/fabrics/fabric", fabric, nil); err != nil {
		return fmt.Errorf("creating SDN fabric %s: %w", fabric.ID, err)
	}
	return nil
}

// SdnFabricUpdate is the body of PUT /cluster/sdn/fabrics/fabric/{id}.
// Delete lists pin field names to clear; per the pin only `area`,
// `redistribute`, and `route_filter` are deletable on OSPF fabrics, and
// `ip_prefix`, `ip6_prefix`, `hello_interval`, `csnp_interval`, and
// `route_filter` on OpenFabric fabrics.
type SdnFabricUpdate struct {
	IPPrefix      string                  `json:"ip_prefix,omitempty"`
	IP6Prefix     string                  `json:"ip6_prefix,omitempty"`
	Area          string                  `json:"area,omitempty"`
	CsnpInterval  *float64                `json:"csnp_interval,omitempty"`
	HelloInterval *float64                `json:"hello_interval,omitempty"`
	RouteFilter   string                  `json:"route_filter,omitempty"`
	Redistribute  []SdnFabricRedistribute `json:"redistribute,omitempty"`
	Digest        string                  `json:"digest,omitempty"`
	Delete        []string                `json:"delete,omitempty"`
}

// UpdateSdnFabric PUTs /cluster/sdn/fabrics/fabric/{id}.
func (c *Client) UpdateSdnFabric(ctx context.Context, id string, upd SdnFabricUpdate) error {
	if err := c.Do(ctx, "PUT", "/cluster/sdn/fabrics/fabric/"+url.PathEscape(id), upd, nil); err != nil {
		return fmt.Errorf("updating SDN fabric %s: %w", id, err)
	}
	return nil
}

// DeleteSdnFabric DELETEs /cluster/sdn/fabrics/fabric/{id}.
func (c *Client) DeleteSdnFabric(ctx context.Context, id string) error {
	if err := c.Do(ctx, "DELETE", "/cluster/sdn/fabrics/fabric/"+url.PathEscape(id), nil, nil); err != nil {
		return fmt.Errorf("deleting SDN fabric %s: %w", id, err)
	}
	return nil
}

// SdnFabricNode is one node member of a fabric
// (/cluster/sdn/fabrics/node/{fabric_id}[/{node_id}]). FabricID, NodeID,
// and Protocol are required on create; Digest is read-only.
type SdnFabricNode struct {
	FabricID   string               `json:"fabric_id"`
	NodeID     string               `json:"node_id"`
	Protocol   string               `json:"protocol,omitempty"`
	IP         string               `json:"ip,omitempty"`
	IP6        string               `json:"ip6,omitempty"`
	Interfaces []SdnFabricInterface `json:"interfaces,omitempty"`
	Digest     string               `json:"digest,omitempty"`
}

// ListSdnFabricNodes enumerates GET /cluster/sdn/fabrics/node/{fabric_id}.
func (c *Client) ListSdnFabricNodes(ctx context.Context, fabricID string) ([]SdnFabricNode, error) {
	var out []SdnFabricNode
	if err := c.Do(ctx, "GET", "/cluster/sdn/fabrics/node/"+url.PathEscape(fabricID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing nodes of SDN fabric %s: %w", fabricID, err)
	}
	return out, nil
}

// GetSdnFabricNode reads GET /cluster/sdn/fabrics/node/{fabric_id}/{node_id}.
func (c *Client) GetSdnFabricNode(ctx context.Context, fabricID, nodeID string) (*SdnFabricNode, error) {
	path := "/cluster/sdn/fabrics/node/" + url.PathEscape(fabricID) + "/" + url.PathEscape(nodeID)
	var out SdnFabricNode
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("reading SDN fabric node %s/%s: %w", fabricID, nodeID, err)
	}
	return &out, nil
}

// CreateSdnFabricNode POSTs /cluster/sdn/fabrics/node/{fabric_id}.
func (c *Client) CreateSdnFabricNode(ctx context.Context, node SdnFabricNode) error {
	path := "/cluster/sdn/fabrics/node/" + url.PathEscape(node.FabricID)
	if err := c.Do(ctx, "POST", path, node, nil); err != nil {
		return fmt.Errorf("adding node %s to SDN fabric %s: %w", node.NodeID, node.FabricID, err)
	}
	return nil
}

// SdnFabricNodeUpdate is the body of PUT
// /cluster/sdn/fabrics/node/{fabric_id}/{node_id}. Delete lists pin field
// names to clear (`interfaces`, `ip`, `ip6`).
type SdnFabricNodeUpdate struct {
	IP         string               `json:"ip,omitempty"`
	IP6        string               `json:"ip6,omitempty"`
	Interfaces []SdnFabricInterface `json:"interfaces,omitempty"`
	Digest     string               `json:"digest,omitempty"`
	Delete     []string             `json:"delete,omitempty"`
}

// UpdateSdnFabricNode PUTs /cluster/sdn/fabrics/node/{fabric_id}/{node_id}.
func (c *Client) UpdateSdnFabricNode(ctx context.Context, fabricID, nodeID string, upd SdnFabricNodeUpdate) error {
	path := "/cluster/sdn/fabrics/node/" + url.PathEscape(fabricID) + "/" + url.PathEscape(nodeID)
	if err := c.Do(ctx, "PUT", path, upd, nil); err != nil {
		return fmt.Errorf("updating SDN fabric node %s/%s: %w", fabricID, nodeID, err)
	}
	return nil
}

// DeleteSdnFabricNode DELETEs
// /cluster/sdn/fabrics/node/{fabric_id}/{node_id}.
func (c *Client) DeleteSdnFabricNode(ctx context.Context, fabricID, nodeID string) error {
	path := "/cluster/sdn/fabrics/node/" + url.PathEscape(fabricID) + "/" + url.PathEscape(nodeID)
	if err := c.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("deleting SDN fabric node %s/%s: %w", fabricID, nodeID, err)
	}
	return nil
}

// SdnFabricRuntimeInterface is one row of GET
// /nodes/{node}/sdn/fabrics/{fabric}/interfaces: a fabric interface with
// its live state as reported by FRR.
type SdnFabricRuntimeInterface struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Type  string `json:"type"`
}

// SdnFabricRuntimeNeighbor is one row of GET
// /nodes/{node}/sdn/fabrics/{fabric}/neighbors.
type SdnFabricRuntimeNeighbor struct {
	Neighbor string `json:"neighbor"`
	Status   string `json:"status"`
	Uptime   string `json:"uptime"`
}

// SdnFabricRuntimeRoute is one row of GET
// /nodes/{node}/sdn/fabrics/{fabric}/routes.
type SdnFabricRuntimeRoute struct {
	Route string   `json:"route"`
	Via   []string `json:"via"`
}

// ListSdnFabricRuntimeInterfaces enumerates GET
// /nodes/{node}/sdn/fabrics/{fabric}/interfaces.
func (c *Client) ListSdnFabricRuntimeInterfaces(ctx context.Context, node, fabricID string) ([]SdnFabricRuntimeInterface, error) {
	var out []SdnFabricRuntimeInterface
	if err := c.Do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/sdn/fabrics/"+url.PathEscape(fabricID)+"/interfaces", nil, &out); err != nil {
		return nil, fmt.Errorf("listing interfaces of SDN fabric %s on %s: %w", fabricID, node, err)
	}
	return out, nil
}

// ListSdnFabricRuntimeNeighbors enumerates GET
// /nodes/{node}/sdn/fabrics/{fabric}/neighbors.
func (c *Client) ListSdnFabricRuntimeNeighbors(ctx context.Context, node, fabricID string) ([]SdnFabricRuntimeNeighbor, error) {
	var out []SdnFabricRuntimeNeighbor
	if err := c.Do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/sdn/fabrics/"+url.PathEscape(fabricID)+"/neighbors", nil, &out); err != nil {
		return nil, fmt.Errorf("listing neighbors of SDN fabric %s on %s: %w", fabricID, node, err)
	}
	return out, nil
}

// ListSdnFabricRuntimeRoutes enumerates GET
// /nodes/{node}/sdn/fabrics/{fabric}/routes.
func (c *Client) ListSdnFabricRuntimeRoutes(ctx context.Context, node, fabricID string) ([]SdnFabricRuntimeRoute, error) {
	var out []SdnFabricRuntimeRoute
	if err := c.Do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/sdn/fabrics/"+url.PathEscape(fabricID)+"/routes", nil, &out); err != nil {
		return nil, fmt.Errorf("listing routes of SDN fabric %s on %s: %w", fabricID, node, err)
	}
	return out, nil
}
