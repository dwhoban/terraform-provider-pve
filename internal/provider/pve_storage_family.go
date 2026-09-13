// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// This file carries the plumbing shared by the network-backed /storage
// resources and data sources (pve_storage_nfs, pve_storage_cifs,
// pve_storage_iscsi, pve_storage_iscsidirect). Each component keeps its
// own model, schema, and CRUD; these helpers cover the attribute set and
// wire conversions every one of them repeats.

// storageFamilyCommonModel holds the attributes shared by every member of
// the network-backed storage family. Family resources and data sources
// embed it in their models.
type storageFamilyCommonModel struct {
	Storage             types.String `tfsdk:"storage"`
	Content             types.Set    `tfsdk:"content"`
	Nodes               types.String `tfsdk:"nodes"`
	Disable             types.Bool   `tfsdk:"disable"`
	Shared              types.Bool   `tfsdk:"shared"`
	PruneBackups        types.String `tfsdk:"prune_backups"`
	MaxProtectedBackups types.Int64  `tfsdk:"max_protected_backups"`
	Digest              types.String `tfsdk:"digest"`
}

// storageFamilyStorageAttribute renders the family's storage identifier
// attribute: required, and forcing recreation on change because the pin's
// update verb has no storage-rename parameter.
func storageFamilyStorageAttribute() schema.Attribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The storage identifier (PVE `pve-storage-id` format). Changing this value forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// storageFamilyCommonAttributes returns the schema attributes common to
// the whole family. contentDescription describes the content set per type;
// type-specific attributes are merged on top by each resource.
func storageFamilyCommonAttributes(contentDescription string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"storage": storageFamilyStorageAttribute(),
		"content": schema.SetAttribute{
			ElementType:         types.StringType,
			Optional:            true,
			MarkdownDescription: contentDescription,
		},
		"nodes": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Comma-separated list of cluster node names the storage configuration applies to (PVE `pve-node-list`).",
		},
		"disable": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Flag to disable the storage.",
		},
		"shared": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Marks the storage as a single storage with the same contents on all nodes (or all nodes listed in `nodes`). It does not make a local storage's contents accessible to other nodes.",
		},
		"prune_backups": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "The retention options in PVE `prune-backups` format, for example `keep-last=3,keep-daily=7`.",
		},
		"max_protected_backups": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Maximal number of protected backups per guest (PVE `max-protected-backups`). Use `-1` for unlimited. Must be -1 or greater.",
			Validators: []validator.Int64{
				int64validator.AtLeast(-1),
			},
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The read-only storage configuration revision (PVE `digest`).",
		},
	}
}

// storageFamilyCommonFromModel projects the shared attributes into a wire
// body. Null/unknown fields are omitted so the request JSON stays minimal.
func storageFamilyCommonFromModel(m storageFamilyCommonModel) pveclient.StorageNet {
	body := pveclient.StorageNet{
		Storage: m.Storage.ValueString(),
	}
	if !m.Content.IsNull() && !m.Content.IsUnknown() {
		body.Content = strings.Join(storageFamilySetToStrings(m.Content), ",")
	}
	if !m.Nodes.IsNull() && !m.Nodes.IsUnknown() {
		body.Nodes = m.Nodes.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		v := m.Disable.ValueBool()
		body.Disable = &v
	}
	if !m.Shared.IsNull() && !m.Shared.IsUnknown() {
		v := m.Shared.ValueBool()
		body.Shared = &v
	}
	if !m.PruneBackups.IsNull() && !m.PruneBackups.IsUnknown() {
		body.PruneBackups = m.PruneBackups.ValueString()
	}
	if !m.MaxProtectedBackups.IsNull() && !m.MaxProtectedBackups.IsUnknown() {
		v := m.MaxProtectedBackups.ValueInt64()
		body.MaxProtectedBackups = &v
	}
	return body
}

// storageFamilyCommonApply writes a fetched configuration into the model;
// absent upstream settings become Terraform nulls so optional attributes
// do not churn between reads and plans.
func storageFamilyCommonApply(s *pveclient.StorageNet, m *storageFamilyCommonModel) {
	m.Storage = types.StringValue(s.Storage)
	m.Content = storageFamilyStringsToSet(storageFamilySplitContent(s.Content))
	m.Nodes = nodeNetworkStringToTF(s.Nodes)
	m.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	m.Shared = nodeNetworkBoolPtrToTF(s.Shared)
	m.PruneBackups = nodeNetworkStringToTF(s.PruneBackups)
	m.MaxProtectedBackups = haInt64PtrToTF(s.MaxProtectedBackups)
	m.Digest = nodeNetworkStringToTF(s.Digest)
}

// storageFamilyCommonDeleteFields returns the PVE wire field names to
// clear on update: optional shared attributes present in state but null in
// the plan.
func storageFamilyCommonDeleteFields(plan, state storageFamilyCommonModel) []string {
	var out []string
	if plan.Content.IsNull() && !state.Content.IsNull() {
		out = append(out, "content")
	}
	if nodeNetworkStringCleared(plan.Nodes, state.Nodes) {
		out = append(out, "nodes")
	}
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	if plan.Shared.IsNull() && !state.Shared.IsNull() {
		out = append(out, "shared")
	}
	if nodeNetworkStringCleared(plan.PruneBackups, state.PruneBackups) {
		out = append(out, "prune-backups")
	}
	if nodeNetworkIntCleared(plan.MaxProtectedBackups, state.MaxProtectedBackups) {
		out = append(out, "max-protected-backups")
	}
	return out
}

// storageFamilyGetChecked reads one storage and errors when the upstream
// type does not match the component's fixed type, so a component never
// silently adopts a storage of another kind.
func storageFamilyGetChecked(ctx context.Context, client *pveclient.Client, storage, wantType string) (*pveclient.StorageNet, error) {
	s, err := client.GetStorageNet(ctx, storage)
	if err != nil {
		return nil, err
	}
	if s.Type != wantType {
		return nil, storageFamilyTypeMismatch(s.Storage, wantType, s.Type)
	}
	return s, nil
}

// storageFamilyTypeMismatch reports an error for a storage whose upstream
// type does not match the fixed type of the component reading it.
func storageFamilyTypeMismatch(storage, want, got string) error {
	return fmt.Errorf("storage %s has upstream type %q, want %q; this component only manages %s storages", storage, got, want, want)
}

// storageFamilySetToStrings flattens a types.Set of strings into a
// []string.
func storageFamilySetToStrings(in types.Set) []string {
	out := make([]string, 0, len(in.Elements()))
	for _, e := range in.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// storageFamilyStringsToSet builds a types.Set of strings from a Go
// []string.
func storageFamilyStringsToSet(in []string) types.Set {
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	set, _ := types.SetValue(types.StringType, elems)
	return set
}

// storageFamilySplitContent decodes the pin's comma-separated
// pve-storage-content-list wire form.
func storageFamilySplitContent(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
