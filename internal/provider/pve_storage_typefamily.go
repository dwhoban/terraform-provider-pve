// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"

	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// storageTypeResource carries the CRUD shared by the block/local storage
// family resources (pve_storage_lvm, pve_storage_lvmthin,
// pve_storage_zfspool, pve_storage_directory). Each component fixes one
// upstream storage `type` via wireType and supplies its own Schema; the
// embed provides Metadata, Configure, and the CRUD verbs against
// /storage and /storage/{storage}.
type storageTypeResource struct {
	client   *pveclient.Client
	suffix   string
	wireType string
}

// storageTypeDataSource carries the read path shared by the block/local
// storage family data sources.
type storageTypeDataSource struct {
	client   *pveclient.Client
	suffix   string
	wireType string
}

// storageTypeModel is the Terraform-facing shape shared by the four block/
// local storage resources and data sources. Only the attributes a
// component's schema exposes are ever populated.
type storageTypeModel struct {
	ID                  types.String                  `tfsdk:"id"`
	Content             types.Set                     `tfsdk:"content"`
	Nodes               types.Set                     `tfsdk:"nodes"`
	Disable             types.Bool                    `tfsdk:"disable"`
	Shared              types.Bool                    `tfsdk:"shared"`
	BWLimit             types.String                  `tfsdk:"bwlimit"`
	PruneBackups        *storageTypePruneBackupsModel `tfsdk:"prune_backups"`
	MaxProtectedBackups types.Int64                   `tfsdk:"max_protected_backups"`

	VGName             types.String `tfsdk:"vgname"`
	Base               types.String `tfsdk:"base"`
	SafeRemove         types.Bool   `tfsdk:"saferemove"`
	SafeRemoveStepSize types.Int64  `tfsdk:"saferemove_stepsize"`
	TaggedOnly         types.Bool   `tfsdk:"tagged_only"`

	ThinPool types.String `tfsdk:"thinpool"`

	Pool      types.String `tfsdk:"pool"`
	BlockSize types.String `tfsdk:"blocksize"`
	Sparse    types.Bool   `tfsdk:"sparse"`

	Path           types.String `tfsdk:"path"`
	IsMountpoint   types.String `tfsdk:"is_mountpoint"`
	CreateBasePath types.Bool   `tfsdk:"create_base_path"`
	CreateSubdirs  types.Bool   `tfsdk:"create_subdirs"`
	Mkdir          types.Bool   `tfsdk:"mkdir"`
}

// storageTypePruneBackupsModel is the retention options block.
type storageTypePruneBackupsModel struct {
	KeepAll     types.Bool  `tfsdk:"keep_all"`
	KeepHourly  types.Int64 `tfsdk:"keep_hourly"`
	KeepDaily   types.Int64 `tfsdk:"keep_daily"`
	KeepWeekly  types.Int64 `tfsdk:"keep_weekly"`
	KeepMonthly types.Int64 `tfsdk:"keep_monthly"`
	KeepYearly  types.Int64 `tfsdk:"keep_yearly"`
	KeepLast    types.Int64 `tfsdk:"keep_last"`
}

// storageTypeAttr maps one Terraform attribute to its PVE wire field name
// for the update `delete` diff.
type storageTypeAttr struct {
	tf   string
	wire string
}

// storageTypeAttrs is the full modeled attribute set; entries for
// attributes a component does not expose simply never differ there.
var storageTypeAttrs = []storageTypeAttr{
	{tf: "content", wire: "content"},
	{tf: "nodes", wire: "nodes"},
	{tf: "disable", wire: "disable"},
	{tf: "shared", wire: "shared"},
	{tf: "bwlimit", wire: "bwlimit"},
	{tf: "prune_backups", wire: "prune-backups"},
	{tf: "max_protected_backups", wire: "max-protected-backups"},
	{tf: "vgname", wire: "vgname"},
	{tf: "base", wire: "base"},
	{tf: "saferemove", wire: "saferemove"},
	{tf: "saferemove_stepsize", wire: "saferemove-stepsize"},
	{tf: "tagged_only", wire: "tagged_only"},
	{tf: "thinpool", wire: "thinpool"},
	{tf: "pool", wire: "pool"},
	{tf: "blocksize", wire: "blocksize"},
	{tf: "sparse", wire: "sparse"},
	{tf: "path", wire: "path"},
	{tf: "is_mountpoint", wire: "is_mountpoint"},
	{tf: "create_base_path", wire: "create-base-path"},
	{tf: "create_subdirs", wire: "create-subdirs"},
	{tf: "mkdir", wire: "mkdir"},
}

// storageTypeStringsToSet builds a types.Set of strings from a Go slice,
// mapping empty input to a null set so unset round-trips as null.
func storageTypeStringsToSet(in []string) types.Set {
	if len(in) == 0 {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	set, _ := types.SetValue(types.StringType, elems)
	return set
}

// storageTypeSetToStrings flattens a types.Set of strings into a []string.
func storageTypeSetToStrings(in types.Set) []string {
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

// storageTypeBoolPtr converts a Terraform bool into a wire pointer, nil
// for null or unknown values.
func storageTypeBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// storageTypeInt64Ptr converts a Terraform int64 into a wire pointer, nil
// for null or unknown values.
func storageTypeInt64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// storageTypeFromModel projects the Terraform model into the wire body;
// null values become nil pointers or empty strings and are omitted from
// the request. The caller sets Type.
func storageTypeFromModel(m storageTypeModel) pveclient.StorageLocalConfig {
	cfg := pveclient.StorageLocalConfig{
		Storage: m.ID.ValueString(),
		Content: storageTypeSetToStrings(m.Content),
		Nodes:   storageTypeSetToStrings(m.Nodes),
		Disable: storageTypeBoolPtr(m.Disable),
		Shared:  storageTypeBoolPtr(m.Shared),
		BWLimit: m.BWLimit.ValueString(),

		MaxProtectedBackups: storageTypeInt64Ptr(m.MaxProtectedBackups),
		VGName:              m.VGName.ValueString(),
		Base:                m.Base.ValueString(),
		SafeRemove:          storageTypeBoolPtr(m.SafeRemove),
		SafeRemoveStepSize:  storageTypeInt64Ptr(m.SafeRemoveStepSize),
		TaggedOnly:          storageTypeBoolPtr(m.TaggedOnly),

		ThinPool: m.ThinPool.ValueString(),

		Pool:      m.Pool.ValueString(),
		BlockSize: m.BlockSize.ValueString(),
		Sparse:    storageTypeBoolPtr(m.Sparse),

		Path:           m.Path.ValueString(),
		IsMountpoint:   m.IsMountpoint.ValueString(),
		CreateBasePath: storageTypeBoolPtr(m.CreateBasePath),
		CreateSubdirs:  storageTypeBoolPtr(m.CreateSubdirs),
		Mkdir:          storageTypeBoolPtr(m.Mkdir),
	}
	if m.PruneBackups != nil {
		cfg.PruneBackups = &pveclient.BackupPruneBackups{
			KeepAll:     storageTypeBoolPtr(m.PruneBackups.KeepAll),
			KeepHourly:  storageTypeInt64Ptr(m.PruneBackups.KeepHourly),
			KeepDaily:   storageTypeInt64Ptr(m.PruneBackups.KeepDaily),
			KeepWeekly:  storageTypeInt64Ptr(m.PruneBackups.KeepWeekly),
			KeepMonthly: storageTypeInt64Ptr(m.PruneBackups.KeepMonthly),
			KeepYearly:  storageTypeInt64Ptr(m.PruneBackups.KeepYearly),
			KeepLast:    storageTypeInt64Ptr(m.PruneBackups.KeepLast),
		}
	}
	return cfg
}

// storageTypeApply writes a fetched configuration into the model; absent
// settings become null.
func storageTypeApply(cfg *pveclient.StorageLocalConfig, m *storageTypeModel) {
	m.ID = types.StringValue(cfg.Storage)
	m.Content = storageTypeStringsToSet(cfg.Content)
	m.Nodes = storageTypeStringsToSet(cfg.Nodes)
	m.Disable = nodeNetworkBoolPtrToTF(cfg.Disable)
	m.Shared = nodeNetworkBoolPtrToTF(cfg.Shared)
	m.BWLimit = nodeNetworkStringToTF(cfg.BWLimit)
	m.MaxProtectedBackups = haInt64PtrToTF(cfg.MaxProtectedBackups)
	m.VGName = nodeNetworkStringToTF(cfg.VGName)
	m.Base = nodeNetworkStringToTF(cfg.Base)
	m.SafeRemove = nodeNetworkBoolPtrToTF(cfg.SafeRemove)
	m.SafeRemoveStepSize = haInt64PtrToTF(cfg.SafeRemoveStepSize)
	m.TaggedOnly = nodeNetworkBoolPtrToTF(cfg.TaggedOnly)
	m.ThinPool = nodeNetworkStringToTF(cfg.ThinPool)
	m.Pool = nodeNetworkStringToTF(cfg.Pool)
	m.BlockSize = nodeNetworkStringToTF(cfg.BlockSize)
	m.Sparse = nodeNetworkBoolPtrToTF(cfg.Sparse)
	m.Path = nodeNetworkStringToTF(cfg.Path)
	m.IsMountpoint = nodeNetworkStringToTF(cfg.IsMountpoint)
	m.CreateBasePath = nodeNetworkBoolPtrToTF(cfg.CreateBasePath)
	m.CreateSubdirs = nodeNetworkBoolPtrToTF(cfg.CreateSubdirs)
	m.Mkdir = nodeNetworkBoolPtrToTF(cfg.Mkdir)
	if cfg.PruneBackups == nil {
		m.PruneBackups = nil
		return
	}
	m.PruneBackups = &storageTypePruneBackupsModel{
		KeepAll:     nodeNetworkBoolPtrToTF(cfg.PruneBackups.KeepAll),
		KeepHourly:  haInt64PtrToTF(cfg.PruneBackups.KeepHourly),
		KeepDaily:   haInt64PtrToTF(cfg.PruneBackups.KeepDaily),
		KeepWeekly:  haInt64PtrToTF(cfg.PruneBackups.KeepWeekly),
		KeepMonthly: haInt64PtrToTF(cfg.PruneBackups.KeepMonthly),
		KeepYearly:  haInt64PtrToTF(cfg.PruneBackups.KeepYearly),
		KeepLast:    haInt64PtrToTF(cfg.PruneBackups.KeepLast),
	}
}

// storageTypeAttrIsSet reports whether a modeled attribute carries
// settings; empty sets count as unset so an emptied list issues a wire
// delete, and an absent prune_backups block counts as unset.
func storageTypeAttrIsSet(m *storageTypeModel, attrName string) bool {
	switch attrName {
	case "id":
		return !m.ID.IsNull()
	case "content":
		return len(m.Content.Elements()) > 0
	case "nodes":
		return len(m.Nodes.Elements()) > 0
	case "disable":
		return !m.Disable.IsNull() && !m.Disable.IsUnknown()
	case "shared":
		return !m.Shared.IsNull() && !m.Shared.IsUnknown()
	case "bwlimit":
		return !m.BWLimit.IsNull() && !m.BWLimit.IsUnknown()
	case "prune_backups":
		return m.PruneBackups != nil
	case "max_protected_backups":
		return !m.MaxProtectedBackups.IsNull() && !m.MaxProtectedBackups.IsUnknown()
	case "vgname":
		return !m.VGName.IsNull() && !m.VGName.IsUnknown()
	case "base":
		return !m.Base.IsNull() && !m.Base.IsUnknown()
	case "saferemove":
		return !m.SafeRemove.IsNull() && !m.SafeRemove.IsUnknown()
	case "saferemove_stepsize":
		return !m.SafeRemoveStepSize.IsNull() && !m.SafeRemoveStepSize.IsUnknown()
	case "tagged_only":
		return !m.TaggedOnly.IsNull() && !m.TaggedOnly.IsUnknown()
	case "thinpool":
		return !m.ThinPool.IsNull() && !m.ThinPool.IsUnknown()
	case "pool":
		return !m.Pool.IsNull() && !m.Pool.IsUnknown()
	case "blocksize":
		return !m.BlockSize.IsNull() && !m.BlockSize.IsUnknown()
	case "sparse":
		return !m.Sparse.IsNull() && !m.Sparse.IsUnknown()
	case "path":
		return !m.Path.IsNull() && !m.Path.IsUnknown()
	case "is_mountpoint":
		return !m.IsMountpoint.IsNull() && !m.IsMountpoint.IsUnknown()
	case "create_base_path":
		return !m.CreateBasePath.IsNull() && !m.CreateBasePath.IsUnknown()
	case "create_subdirs":
		return !m.CreateSubdirs.IsNull() && !m.CreateSubdirs.IsUnknown()
	case "mkdir":
		return !m.Mkdir.IsNull() && !m.Mkdir.IsUnknown()
	default:
		return false
	}
}

// storageTypeDeleteFields returns the PVE wire field names to clear on
// update: attributes present in state but unset in the plan.
func storageTypeDeleteFields(plan, state storageTypeModel) []string {
	var out []string
	for _, a := range storageTypeAttrs {
		if !storageTypeAttrIsSet(&plan, a.tf) && storageTypeAttrIsSet(&state, a.tf) {
			out = append(out, a.wire)
		}
	}
	return out
}

// storageTypeGetChecked reads /storage/{storage} and rejects a document
// whose upstream `type` does not match the component's fixed type.
func storageTypeGetChecked(ctx context.Context, client *pveclient.Client, id, wantType string) (*pveclient.StorageLocalConfig, error) {
	cfg, err := client.GetStorageLocal(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg.Type != wantType {
		return nil, fmt.Errorf("storage %s has upstream type %q, want %q; this component only manages %s storages", id, cfg.Type, wantType, wantType)
	}
	return cfg, nil
}

// storageTypeIsMissing reports whether err means the storage configuration
// is absent: a 404, or PVE's "no such storage" HTTP 500 for unknown IDs.
func storageTypeIsMissing(err error) bool {
	if err == nil {
		return false
	}
	if isPVEClientNotFound(err) {
		return true
	}
	var apiErr *pveclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 500 && strings.Contains(strings.Join(apiErr.Errors, "; "), "no such storage") {
		return true
	}
	return false
}

// storageTypePruneBackupsAttrs builds the retention options attribute set.
func storageTypePruneBackupsAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"keep_all": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Keep all backups; conflicts with the other options.",
		},
		"keep_hourly": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep backups for the last N hours.",
		},
		"keep_daily": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep backups for the last N days.",
		},
		"keep_weekly": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep backups for the last N weeks.",
		},
		"keep_monthly": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep backups for the last N months.",
		},
		"keep_yearly": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep backups for the last N years.",
		},
		"keep_last": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Keep the last N backups.",
		},
	}
}

// storageTypeSharedResourceAttrs builds the attributes every block/local
// storage resource carries; contentAllowed names the content values that
// make sense for the storage type in the schema description.
func storageTypeSharedResourceAttrs(contentAllowed string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The storage identifier (PVE `pve-storage-id` format). Changing this value forces recreation.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"content": schema.SetAttribute{
			ElementType:         types.StringType,
			Optional:            true,
			MarkdownDescription: "Allowed content types. Accepted values for this storage type: " + contentAllowed + ". PVE validates the set upstream.",
		},
		"nodes": schema.SetAttribute{
			ElementType:         types.StringType,
			Optional:            true,
			MarkdownDescription: "List of nodes for which the storage configuration applies.",
		},
		"disable": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Flag to disable the storage.",
		},
		"shared": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Indicate that this is a single storage with the same contents on all nodes (or all listed in `nodes`). It will not make the contents of a local storage automatically accessible to other nodes; it just marks an already shared storage as such.",
		},
		"bwlimit": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "I/O bandwidth limit property string in KiB/s, for example `default=500,restore=100`. Keys: `clone`, `default`, `migration`, `move`, `restore`.",
		},
		"prune_backups": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: "Backup retention options overriding a backup job's settings. Writing the block replaces the whole group on the PVE side. `keep_all = true` conflicts with the other options.",
			Attributes:          storageTypePruneBackupsAttrs(),
		},
		"max_protected_backups": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Maximal number of protected backups per guest; use `-1` for unlimited (PVE default: unlimited for users with `Datastore.Allocate`, `5` for other users). Must be -1 or greater.",
			Validators: []validator.Int64{
				int64validator.AtLeast(-1),
			},
		},
	}
}

// storageTypeSharedDataSourceAttrs builds the read-only attribute set the
// four block/local storage data sources carry. The framework's resource
// and data-source schema types are distinct, so the shared descriptions
// are restated with data-source types.
func storageTypeSharedDataSourceAttrs(contentAllowed string) map[string]dsschema.Attribute {
	return map[string]dsschema.Attribute{
		"id": dsschema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The storage identifier to read.",
		},
		"content": dsschema.SetAttribute{
			ElementType:         types.StringType,
			Computed:            true,
			MarkdownDescription: "Allowed content types. Accepted values for this storage type: " + contentAllowed + ". PVE validates the set upstream.",
		},
		"nodes": dsschema.SetAttribute{
			ElementType:         types.StringType,
			Computed:            true,
			MarkdownDescription: "List of nodes for which the storage configuration applies.",
		},
		"disable": dsschema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Flag to disable the storage.",
		},
		"shared": dsschema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Indicate that this is a single storage with the same contents on all nodes (or all listed in `nodes`). It will not make the contents of a local storage automatically accessible to other nodes; it just marks an already shared storage as such.",
		},
		"bwlimit": dsschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "I/O bandwidth limit property string in KiB/s, for example `default=500,restore=100`. Keys: `clone`, `default`, `migration`, `move`, `restore`.",
		},
		"prune_backups": dsschema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Backup retention options overriding a backup job's settings. `keep_all = true` conflicts with the other options.",
			Attributes: map[string]dsschema.Attribute{
				"keep_all": dsschema.BoolAttribute{
					Computed:            true,
					MarkdownDescription: "Keep all backups; conflicts with the other options.",
				},
				"keep_hourly": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep backups for the last N hours.",
				},
				"keep_daily": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep backups for the last N days.",
				},
				"keep_weekly": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep backups for the last N weeks.",
				},
				"keep_monthly": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep backups for the last N months.",
				},
				"keep_yearly": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep backups for the last N years.",
				},
				"keep_last": dsschema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Keep the last N backups.",
				},
			},
		},
		"max_protected_backups": dsschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal number of protected backups per guest; `-1` means unlimited (PVE default: unlimited for users with `Datastore.Allocate`, `5` for other users).",
		},
	}
}

// Metadata implements resource.Resource.
func (r *storageTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.suffix
}

// Configure implements resource.ResourceWithConfigure.
func (r *storageTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *storageTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageTypeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating "+r.fullName(), "provider client is not configured")
		return
	}
	cfg := storageTypeFromModel(plan)
	cfg.Type = r.wireType
	if err := r.client.CreateStorageLocal(ctx, cfg); err != nil {
		resp.Diagnostics.AddError(
			"Error creating "+r.fullName(),
			fmt.Sprintf("creating storage %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading "+r.fullName()+" after create",
			fmt.Sprintf("reading storage %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE storage", map[string]any{"id": plan.ID.ValueString(), "type": r.wireType})
}

// Read implements resource.Resource.
func (r *storageTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state storageTypeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if storageTypeIsMissing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading "+r.fullName(),
			fmt.Sprintf("reading storage %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *storageTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan storageTypeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state storageTypeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := storageTypeFromModel(plan)
	cfg.Type = r.wireType
	deleteFields := storageTypeDeleteFields(plan, state)
	if err := r.client.UpdateStorageLocal(ctx, plan.ID.ValueString(), cfg, deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating "+r.fullName(),
			fmt.Sprintf("updating storage %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading "+r.fullName()+" after update",
			fmt.Sprintf("reading storage %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE storage", map[string]any{"id": plan.ID.ValueString(), "type": r.wireType})
}

// Delete implements resource.Resource.
func (r *storageTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageTypeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE storage", map[string]any{"id": state.ID.ValueString(), "type": r.wireType})
	if err := r.client.DeleteStorageLocal(ctx, state.ID.ValueString()); err != nil {
		if storageTypeIsMissing(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting "+r.fullName(),
			fmt.Sprintf("deleting storage %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *storageTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid "+r.fullName()+" import ID", "import ID must be the storage identifier, e.g. `local`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE, checking the upstream type.
func (r *storageTypeResource) readInto(ctx context.Context, m *storageTypeModel) error {
	cfg, err := storageTypeGetChecked(ctx, r.client, m.ID.ValueString(), r.wireType)
	if err != nil {
		return err
	}
	storageTypeApply(cfg, m)
	return nil
}

// fullName names the resource component in diagnostics, e.g. pve_storage_lvm.
func (r *storageTypeResource) fullName() string {
	return "pve_" + r.suffix
}

// Metadata implements datasource.DataSource.
func (d *storageTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.suffix
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *storageTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *storageTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state storageTypeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading "+d.fullName(), "provider client is not configured")
		return
	}
	cfg, err := storageTypeGetChecked(ctx, d.client, state.ID.ValueString(), d.wireType)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading "+d.fullName(),
			fmt.Sprintf("reading storage %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	storageTypeApply(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// fullName names the data source component in diagnostics.
func (d *storageTypeDataSource) fullName() string {
	return "pve_" + d.suffix
}
