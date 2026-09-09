// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// sdnPrefixListEntryModel mirrors one entry inside the ordered
// `entries` list of the SDN prefix list resources. Seq is the
// server-assigned position handle, resynced after every apply.
type sdnPrefixListEntryModel struct {
	Seq    types.Int64  `tfsdk:"seq"`
	Action types.String `tfsdk:"action"`
	Prefix types.String `tfsdk:"prefix"`
	Ge     types.Int64  `tfsdk:"ge"`
	Le     types.Int64  `tfsdk:"le"`
}

// sdnPrefixListEntryAttributes renders the nested entry object schema,
// following the pin's prefix-list entry parameter block.
func sdnPrefixListEntryAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"seq": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Entry sequence number assigned by PVE (1-4294967295). The sequence orders entry evaluation, so the provider resyncs it from PVE after every apply; the list order in configuration is authoritative.",
		},
		"action": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf("permit", "deny"),
			},
			MarkdownDescription: "What to do with matching routes. Must be one of: `permit`, `deny`.",
		},
		"prefix": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The IP or IPv6 prefix to match, in CIDR notation (e.g. `10.0.0.0/8`, `fd00::/8`).",
		},
		"ge": schema.Int64Attribute{
			Optional: true,
			Validators: []validator.Int64{
				int64validator.Between(0, 128),
			},
			MarkdownDescription: "Minimum prefix length to match. Must be between 0 and 128.",
		},
		"le": schema.Int64Attribute{
			Optional: true,
			Validators: []validator.Int64{
				int64validator.Between(0, 128),
			},
			MarkdownDescription: "Maximum prefix length to match. Must be between 0 and 128.",
		},
	}
}

// sdnPrefixListEntryDataSourceAttributes renders the entry object
// schema for data sources, where every field is computed.
func sdnPrefixListEntryDataSourceAttributes() map[string]datasourceschema.Attribute {
	return map[string]datasourceschema.Attribute{
		"seq": datasourceschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Entry sequence number assigned by PVE (1-4294967295); orders entry evaluation.",
		},
		"action": datasourceschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "What to do with matching routes: `permit` or `deny`.",
		},
		"prefix": datasourceschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The matched IP or IPv6 prefix, in CIDR notation.",
		},
		"ge": datasourceschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Minimum prefix length to match; null when unset.",
		},
		"le": datasourceschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximum prefix length to match; null when unset.",
		},
	}
}

// sdnRouteMapEntryDataSourceAttributes renders the route map entry
// object schema for data sources, where every field is computed.
func sdnRouteMapEntryDataSourceAttributes() map[string]datasourceschema.Attribute {
	return map[string]datasourceschema.Attribute{
		"order": datasourceschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Entry index assigned by PVE (0-65535); orders entry evaluation.",
		},
		"action": datasourceschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Matching policy of the entry: `permit` or `deny`.",
		},
		"call": datasourceschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Route map jumped to when the entry matches; null when unset.",
		},
		"exit_action": datasourceschema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "What to do after the entry matches; the pin's `exit-action` field.",
			Attributes: map[string]datasourceschema.Attribute{
				"key": datasourceschema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Exit behavior: `on-match-goto`, `on-match-next`, or `continue`.",
				},
				"value": datasourceschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Target entry index for `on-match-goto`; null when unset.",
				},
			},
		},
		"match": datasourceschema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Ordered match clauses; the route must satisfy every clause.",
			NestedObject: datasourceschema.NestedAttributeObject{
				Attributes: map[string]datasourceschema.Attribute{
					"key": datasourceschema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Route property to match.",
					},
					"value": datasourceschema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Value the key matches on; key-dependent.",
					},
				},
			},
		},
		"set": datasourceschema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Ordered set clauses applied to matching routes.",
			NestedObject: datasourceschema.NestedAttributeObject{
				Attributes: map[string]datasourceschema.Attribute{
					"key": datasourceschema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Route property to set.",
					},
					"value": datasourceschema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Value to set the key to; key-dependent.",
					},
				},
			},
		},
	}
}

// sdnPrefixListEntriesFromModel projects planned entry models into wire
// entries; null attributes carry the zero value, which the wire marshal
// omits from the request body.
func sdnPrefixListEntriesFromModel(in []sdnPrefixListEntryModel) []pveclient.SdnPrefixListEntry {
	out := make([]pveclient.SdnPrefixListEntry, 0, len(in))
	for _, m := range in {
		out = append(out, pveclient.SdnPrefixListEntry{
			Action: m.Action.ValueString(),
			Prefix: m.Prefix.ValueString(),
			Ge:     m.Ge.ValueInt64Pointer(),
			Le:     m.Le.ValueInt64Pointer(),
		})
	}
	return out
}

// sdnPrefixListEntriesToModel projects wire entries into entry models
// with fresh sequence numbers.
func sdnPrefixListEntriesToModel(in []pveclient.SdnPrefixListEntry) []sdnPrefixListEntryModel {
	out := make([]sdnPrefixListEntryModel, 0, len(in))
	for _, e := range in {
		out = append(out, sdnPrefixListEntryModel{
			Seq:    firewallRulesInt64TF(e.Seq),
			Action: types.StringValue(e.Action),
			Prefix: types.StringValue(e.Prefix),
			Ge:     firewallRulesInt64TF(e.Ge),
			Le:     firewallRulesInt64TF(e.Le),
		})
	}
	return out
}

// sdnPrefixListEntriesSame reports whether two entries carry the same
// user-managed fields, ignoring the server-assigned seq.
func sdnPrefixListEntriesSame(a, b pveclient.SdnPrefixListEntry) bool {
	if a.Action != b.Action || a.Prefix != b.Prefix {
		return false
	}
	return sdnListInt64PtrEqual(a.Ge, b.Ge) && sdnListInt64PtrEqual(a.Le, b.Le)
}

// sdnPrefixListEntrySeq extracts an entry's upstream sequence number,
// refusing entries the server returned without one.
func sdnPrefixListEntrySeq(e pveclient.SdnPrefixListEntry) (int64, error) {
	if e.Seq == nil {
		return 0, fmt.Errorf("server returned a prefix list entry without seq (prefix %q)", e.Prefix)
	}
	return *e.Seq, nil
}

// sdnPrefixListClearedFields names the optional entry fields present in
// existing but absent in desired, for PVE's `delete` query parameter.
func sdnPrefixListClearedFields(existing, desired pveclient.SdnPrefixListEntry) []string {
	var fields []string
	if existing.Ge != nil && desired.Ge == nil {
		fields = append(fields, "ge")
	}
	if existing.Le != nil && desired.Le == nil {
		fields = append(fields, "le")
	}
	return fields
}

// sdnRouteMapKVModel mirrors one match or set clause of a route map
// entry.
type sdnRouteMapKVModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

// sdnRouteMapExitActionModel mirrors a route map entry's exit action.
type sdnRouteMapExitActionModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.Int64  `tfsdk:"value"`
}

// sdnRouteMapEntryModel mirrors one entry inside the ordered `entries`
// list of the SDN route map resources. Order is the server-assigned
// position handle, resynced after every apply.
type sdnRouteMapEntryModel struct {
	Order      types.Int64                 `tfsdk:"order"`
	Action     types.String                `tfsdk:"action"`
	Call       types.String                `tfsdk:"call"`
	ExitAction *sdnRouteMapExitActionModel `tfsdk:"exit_action"`
	Match      []sdnRouteMapKVModel        `tfsdk:"match"`
	Set        []sdnRouteMapKVModel        `tfsdk:"set"`
}

// sdnRouteMapEntryAttributes renders the nested entry object schema,
// following the pin's route map entry parameter block.
func sdnRouteMapEntryAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"order": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Entry index assigned by PVE (0-65535). The index orders entry evaluation, so the provider resyncs it from PVE after every apply; the list order in configuration is authoritative. New entries are appended after the highest existing index.",
		},
		"action": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf("permit", "deny"),
			},
			MarkdownDescription: "Matching policy of the entry. Must be one of: `permit`, `deny`.",
		},
		"call": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Jump to the route map with this identifier when the entry matches.",
		},
		"exit_action": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "What to do after the entry matches; the pin's `exit-action` parameter.",
			Attributes: map[string]schema.Attribute{
				"key": schema.StringAttribute{
					Required: true,
					Validators: []validator.String{
						stringvalidator.OneOf("on-match-goto", "on-match-next", "continue"),
					},
					MarkdownDescription: "Exit behavior. Must be one of: `on-match-goto`, `on-match-next`, `continue`.",
				},
				"value": schema.Int64Attribute{
					Optional: true,
					Validators: []validator.Int64{
						int64validator.Between(0, 65535),
					},
					MarkdownDescription: "Target entry index for `on-match-goto`. Must be between 0 and 65535.",
				},
			},
		},
		"match": schema.ListNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Ordered match clauses; the route must satisfy every clause. Values are key-dependent (e.g. a prefix list name for `ip-address-prefix-list`, a community for `tag`).",
			NestedObject: schema.NestedAttributeObject{
				Attributes: sdnRouteMapMatchAttributes(),
			},
		},
		"set": schema.ListNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Ordered set clauses applied to matching routes (e.g. set `local-preference` to `200`). Values are key-dependent.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: sdnRouteMapSetAttributes(),
			},
		},
	}
}

// sdnRouteMapMatchKeys enumerates the pin's match clause keys.
var sdnRouteMapMatchKeys = []string{
	"route-type", "vni",
	"ip-address-prefix-list", "ip6-address-prefix-list",
	"ip-next-hop-prefix-list", "ip6-next-hop-prefix-list",
	"ip-next-hop-address", "ip6-next-hop-address",
	"metric", "local-preference", "peer", "tag",
}

// sdnRouteMapSetKeys enumerates the pin's set clause keys.
var sdnRouteMapSetKeys = []string{
	"ip-next-hop-peer-address", "ip-next-hop", "ip-next-hop-unchanged",
	"ip6-next-hop-peer-address", "ip6-next-hop-prefer-global", "ip6-next-hop",
	"local-preference", "tag", "weight", "metric", "src",
}

// sdnRouteMapMatchAttributes renders the match clause object schema.
func sdnRouteMapMatchAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"key": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf(sdnRouteMapMatchKeys...),
			},
			MarkdownDescription: "Route property to match. Must be one of: `route-type`, `vni`, `ip-address-prefix-list`, `ip6-address-prefix-list`, `ip-next-hop-prefix-list`, `ip6-next-hop-prefix-list`, `ip-next-hop-address`, `ip6-next-hop-address`, `metric`, `local-preference`, `peer`, `tag`.",
		},
		"value": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Value the key should match on; key-dependent (e.g. a prefix list name, an address, a route type like `external`).",
		},
	}
}

// sdnRouteMapSetAttributes renders the set clause object schema.
func sdnRouteMapSetAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"key": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf(sdnRouteMapSetKeys...),
			},
			MarkdownDescription: "Route property to set. Must be one of: `ip-next-hop-peer-address`, `ip-next-hop`, `ip-next-hop-unchanged`, `ip6-next-hop-peer-address`, `ip6-next-hop-prefer-global`, `ip6-next-hop`, `local-preference`, `tag`, `weight`, `metric`, `src`.",
		},
		"value": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Value to set the key to; key-dependent (e.g. `200` for `local-preference`, a next-hop address for `ip-next-hop`).",
		},
	}
}

// sdnRouteMapKVsFromModel projects KV models into wire clauses.
func sdnRouteMapKVsFromModel(in []sdnRouteMapKVModel) []pveclient.SdnRouteMapKV {
	out := make([]pveclient.SdnRouteMapKV, 0, len(in))
	for _, m := range in {
		out = append(out, pveclient.SdnRouteMapKV{
			Key:   m.Key.ValueString(),
			Value: m.Value.ValueString(),
		})
	}
	return out
}

// sdnRouteMapKVsToModel projects wire clauses into KV models.
func sdnRouteMapKVsToModel(in []pveclient.SdnRouteMapKV) []sdnRouteMapKVModel {
	out := make([]sdnRouteMapKVModel, 0, len(in))
	for _, kv := range in {
		out = append(out, sdnRouteMapKVModel{
			Key:   types.StringValue(kv.Key),
			Value: nodeNetworkStringToTF(kv.Value),
		})
	}
	return out
}

// sdnRouteMapEntriesFromModel projects planned entry models into wire
// entries; null optional fields carry their zero value, which the wire
// marshal omits from the request body.
func sdnRouteMapEntriesFromModel(in []sdnRouteMapEntryModel) []pveclient.SdnRouteMapEntry {
	out := make([]pveclient.SdnRouteMapEntry, 0, len(in))
	for _, m := range in {
		e := pveclient.SdnRouteMapEntry{
			Action: m.Action.ValueString(),
			Call:   m.Call.ValueString(),
			Match:  sdnRouteMapKVsFromModel(m.Match),
			Set:    sdnRouteMapKVsFromModel(m.Set),
		}
		if m.ExitAction != nil {
			e.ExitAction = &pveclient.SdnRouteMapExitAction{
				Key:   m.ExitAction.Key.ValueString(),
				Value: m.ExitAction.Value.ValueInt64Pointer(),
			}
		}
		out = append(out, e)
	}
	return out
}

// sdnRouteMapEntriesToModel projects wire entries into entry models
// with fresh order indexes.
func sdnRouteMapEntriesToModel(in []pveclient.SdnRouteMapEntry) []sdnRouteMapEntryModel {
	out := make([]sdnRouteMapEntryModel, 0, len(in))
	for _, e := range in {
		m := sdnRouteMapEntryModel{
			Order:  firewallRulesInt64TF(e.Order),
			Action: types.StringValue(e.Action),
			Call:   nodeNetworkStringToTF(e.Call),
			Match:  sdnRouteMapKVsToModel(e.Match),
			Set:    sdnRouteMapKVsToModel(e.Set),
		}
		if e.ExitAction != nil {
			m.ExitAction = &sdnRouteMapExitActionModel{
				Key:   types.StringValue(e.ExitAction.Key),
				Value: firewallRulesInt64TF(e.ExitAction.Value),
			}
		}
		out = append(out, m)
	}
	return out
}

// sdnRouteMapEntriesSame reports whether two entries carry the same
// user-managed fields, ignoring the server-assigned order and the
// concurrency digest.
func sdnRouteMapEntriesSame(a, b pveclient.SdnRouteMapEntry) bool {
	if a.Action != b.Action || a.Call != b.Call {
		return false
	}
	if (a.ExitAction == nil) != (b.ExitAction == nil) {
		return false
	}
	if a.ExitAction != nil {
		if a.ExitAction.Key != b.ExitAction.Key || !sdnListInt64PtrEqual(a.ExitAction.Value, b.ExitAction.Value) {
			return false
		}
	}
	return sdnRouteMapKVsSame(a.Match, b.Match) && sdnRouteMapKVsSame(a.Set, b.Set)
}

// sdnRouteMapKVsSame compares two clause lists element-wise; clause
// order is significant to route evaluation.
func sdnRouteMapKVsSame(a, b []pveclient.SdnRouteMapKV) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sdnRouteMapEntryOrder extracts an entry's upstream order index,
// refusing entries the server returned without one.
func sdnRouteMapEntryOrder(e pveclient.SdnRouteMapEntry) (int64, error) {
	if e.Order == nil {
		return 0, fmt.Errorf("server returned a route map entry without order (action %q)", e.Action)
	}
	return *e.Order, nil
}

// sdnRouteMapClearedFields names the optional entry fields present in
// existing but absent in desired, for PVE's `delete` query parameter.
func sdnRouteMapClearedFields(existing, desired pveclient.SdnRouteMapEntry) []string {
	var fields []string
	if existing.Call != "" && desired.Call == "" {
		fields = append(fields, "call")
	}
	if existing.ExitAction != nil && desired.ExitAction == nil {
		fields = append(fields, "exit-action")
	}
	if len(existing.Match) > 0 && len(desired.Match) == 0 {
		fields = append(fields, "match")
	}
	if len(existing.Set) > 0 && len(desired.Set) == 0 {
		fields = append(fields, "set")
	}
	return fields
}

// sdnListInt64PtrEqual compares two optional integers, treating nil as absent.
func sdnListInt64PtrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// sdnListOps abstracts the wire operations the shared ordered-diff
// engine needs for one ordered SDN entry collection, mirroring the
// firewall rules family's re-list-per-mutation discipline.
type sdnListOps[E any] interface {
	// List returns the entries in upstream order.
	List(ctx context.Context) ([]E, error)
	// Position extracts an entry's upstream position handle.
	Position(entry E) (int64, error)
	// Same reports whether existing and desired carry the same
	// user-managed fields, ignoring the position handle.
	Same(existing, desired E) bool
	// Create appends one entry; PVE assigns its position.
	Create(ctx context.Context, desired E) error
	// Update rewrites the entry at position in place.
	Update(ctx context.Context, position int64, existing, desired E) error
	// Delete removes the entry at position.
	Delete(ctx context.Context, position int64) error
}

// sdnListApplyDiff converges the upstream entry collection to the
// planned entries, in order. It never trusts a position across calls:
// surplus entries are deleted highest position first with a re-list
// after every delete, and each remaining plan entry is compared against
// a freshly listed collection before being written in place (Update) or
// appended (Create), so an upstream that renumbers on any mutation
// stays consistent. A final re-list returns the entries with fresh
// positions.
func sdnListApplyDiff[E any](ctx context.Context, ops sdnListOps[E], plan []E) ([]E, error) {
	current, err := ops.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing SDN list entries: %w", err)
	}
	// 1. Remove surplus entries, highest position first.
	for len(current) > len(plan) {
		position, err := ops.Position(current[len(current)-1])
		if err != nil {
			return nil, err
		}
		if err := ops.Delete(ctx, position); err != nil {
			return nil, fmt.Errorf("deleting SDN list entry at position %d: %w", position, err)
		}
		if current, err = ops.List(ctx); err != nil {
			return nil, fmt.Errorf("re-listing SDN list entries after delete: %w", err)
		}
	}
	// 2. Write each plan entry, one at a time, against a fresh listing:
	// entries that already match are kept, entries below the current
	// length are modified in place, entries beyond it are appended.
	for i := range plan {
		if current, err = ops.List(ctx); err != nil {
			return nil, fmt.Errorf("re-listing SDN list entries: %w", err)
		}
		if i < len(current) {
			if ops.Same(current[i], plan[i]) {
				continue
			}
			position, err := ops.Position(current[i])
			if err != nil {
				return nil, err
			}
			if err := ops.Update(ctx, position, current[i], plan[i]); err != nil {
				return nil, fmt.Errorf("updating SDN list entry at position %d: %w", position, err)
			}
			continue
		}
		if err := ops.Create(ctx, plan[i]); err != nil {
			return nil, fmt.Errorf("creating SDN list entry at index %d: %w", i, err)
		}
	}
	// 3. Resync positions into state.
	fresh, err := ops.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("re-listing SDN list entries: %w", err)
	}
	return fresh, nil
}

// sdnListDeleteAll empties the collection, deleting from the highest
// position downward and re-listing between deletions so upstream
// renumbering never invalidates a position.
func sdnListDeleteAll[E any](ctx context.Context, ops sdnListOps[E]) error {
	for {
		current, err := ops.List(ctx)
		if err != nil {
			return fmt.Errorf("listing SDN list entries: %w", err)
		}
		if len(current) == 0 {
			return nil
		}
		position, err := ops.Position(current[len(current)-1])
		if err != nil {
			return err
		}
		if err := ops.Delete(ctx, position); err != nil {
			return fmt.Errorf("deleting SDN list entry at position %d: %w", position, err)
		}
	}
}

// sdnPrefixListOps adapts one prefix list's entries to sdnListOps.
type sdnPrefixListOps struct {
	client *pveclient.Client
	id     string
}

// List implements sdnListOps.
func (o sdnPrefixListOps) List(ctx context.Context) ([]pveclient.SdnPrefixListEntry, error) {
	return o.client.ListSdnPrefixListEntries(ctx, o.id)
}

// Position implements sdnListOps.
func (o sdnPrefixListOps) Position(entry pveclient.SdnPrefixListEntry) (int64, error) {
	return sdnPrefixListEntrySeq(entry)
}

// Same implements sdnListOps.
func (o sdnPrefixListOps) Same(existing, desired pveclient.SdnPrefixListEntry) bool {
	return sdnPrefixListEntriesSame(existing, desired)
}

// Create implements sdnListOps; seq is omitted so PVE assigns the
// position.
func (o sdnPrefixListOps) Create(ctx context.Context, desired pveclient.SdnPrefixListEntry) error {
	return o.client.CreateSdnPrefixListEntry(ctx, o.id, desired)
}

// Update implements sdnListOps.
func (o sdnPrefixListOps) Update(ctx context.Context, position int64, existing, desired pveclient.SdnPrefixListEntry) error {
	return o.client.UpdateSdnPrefixListEntry(ctx, o.id, position, desired, sdnPrefixListClearedFields(existing, desired))
}

// Delete implements sdnListOps.
func (o sdnPrefixListOps) Delete(ctx context.Context, position int64) error {
	return o.client.DeleteSdnPrefixListEntry(ctx, o.id, position)
}

// sdnRouteMapOps adapts one route map's entries to sdnListOps.
type sdnRouteMapOps struct {
	client     *pveclient.Client
	routeMapID string
}

// List implements sdnListOps. A route map materializes only through its
// entries (the pin defines no route-map creation verb), so a missing
// collection reads as empty rather than failing the diff.
func (o sdnRouteMapOps) List(ctx context.Context) ([]pveclient.SdnRouteMapEntry, error) {
	entries, err := o.client.ListSdnRouteMapEntries(ctx, o.routeMapID)
	if isPVEClientNotFound(err) {
		return []pveclient.SdnRouteMapEntry{}, nil
	}
	return entries, err
}

// Position implements sdnListOps.
func (o sdnRouteMapOps) Position(entry pveclient.SdnRouteMapEntry) (int64, error) {
	return sdnRouteMapEntryOrder(entry)
}

// Same implements sdnListOps.
func (o sdnRouteMapOps) Same(existing, desired pveclient.SdnRouteMapEntry) bool {
	return sdnRouteMapEntriesSame(existing, desired)
}

// Create implements sdnListOps; the order index (required by the pin's
// create verb) is placed after the highest existing index so appended
// entries never collide with in-place entries.
func (o sdnRouteMapOps) Create(ctx context.Context, desired pveclient.SdnRouteMapEntry) error {
	current, err := o.client.ListSdnRouteMapEntries(ctx, o.routeMapID)
	if isPVEClientNotFound(err) {
		current = nil
	} else if err != nil {
		return err
	}
	// Start below every possible order so an empty map places its
	// first entry at 0; otherwise step past the true maximum.
	highest := int64(-10)
	for _, e := range current {
		if e.Order != nil && *e.Order > highest {
			highest = *e.Order
		}
	}
	desired.Order = pveclient.SdnRouteMapOrderPtr(highest + 10)
	return o.client.CreateSdnRouteMapEntry(ctx, desired)
}

// Update implements sdnListOps.
func (o sdnRouteMapOps) Update(ctx context.Context, position int64, existing, desired pveclient.SdnRouteMapEntry) error {
	return o.client.UpdateSdnRouteMapEntry(ctx, o.routeMapID, position, desired, sdnRouteMapClearedFields(existing, desired))
}

// Delete implements sdnListOps.
func (o sdnRouteMapOps) Delete(ctx context.Context, position int64) error {
	return o.client.DeleteSdnRouteMapEntry(ctx, o.routeMapID, position)
}

// sdnListConfigureResource extracts the shared client from provider
// data for the SDN list resources. Nil provider data leaves the
// resource unconfigured (unit tests).
func sdnListConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}
