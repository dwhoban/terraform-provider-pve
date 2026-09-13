// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveRoleResource{}
	_ resource.ResourceWithConfigure   = &pveRoleResource{}
	_ resource.ResourceWithImportState = &pveRoleResource{}
)

// NewPveRoleResource returns the resource implementation.
func NewPveRoleResource() resource.Resource {
	return &pveRoleResource{}
}

// pveRoleResource manages a custom PVE role via /access/roles.
type pveRoleResource struct {
	client *pveclient.Client
}

// pveRoleResourceModel is the Terraform-facing shape.
type pveRoleResourceModel struct {
	RoleID types.String `tfsdk:"roleid"`
	Privs  types.Set    `tfsdk:"privs"`
}

// Metadata implements resource.Resource.
func (r *pveRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRole
}

// Schema implements resource.Resource.
func (r *pveRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom role in Proxmox VE (`/access/roles`). Built-in roles (those the role index flags as `special`, e.g. `PVEVMAdmin`) cannot be created, modified, or deleted by PVE; this resource is for user-defined roles, while `pve_role` (data source) can read any role.",
		Attributes: map[string]schema.Attribute{
			"roleid": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Role identifier (PVE `pve-roleid` format). Changing it forces replacement.",
			},
			"privs": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of PVE privileges granted by this role. Updates replace the full privilege set; the API's `append` merge flag is not modeled. An empty set defines a role without privileges. PVE validates privilege names itself, so newer releases may accept privileges beyond the known list: " +
					"`Datastore.Allocate`, `Datastore.AllocateSpace`, `Datastore.AllocateTemplate`, `Datastore.Audit`, " +
					"`Group.Allocate`, `Mapping.Audit`, `Mapping.Modify`, `Mapping.Use`, " +
					"`Permissions.Modify`, `Pool.Allocate`, `Pool.Audit`, " +
					"`Realm.Allocate`, `Realm.AllocateUser`, " +
					"`SDN.Allocate`, `SDN.Audit`, `SDN.Use`, " +
					"`Sys.AccessNetwork`, `Sys.Audit`, `Sys.Console`, `Sys.Incoming`, `Sys.Modify`, `Sys.PowerMgmt`, `Sys.Syslog`, " +
					"`User.Modify`, " +
					"`VM.Allocate`, `VM.Audit`, `VM.Backup`, `VM.Clone`, " +
					"`VM.Config.CDROM`, `VM.Config.CPU`, `VM.Config.Cloudinit`, `VM.Config.Disk`, `VM.Config.HWType`, `VM.Config.Memory`, `VM.Config.Network`, `VM.Config.Options`, " +
					"`VM.Console`, `VM.GuestAgent.Audit`, `VM.GuestAgent.FileRead`, `VM.GuestAgent.FileSystemMgmt`, `VM.GuestAgent.FileWrite`, `VM.GuestAgent.Unrestricted`, " +
					"`VM.Migrate`, `VM.PowerMgmt`, `VM.Replicate`, `VM.Snapshot`, `VM.Snapshot.Rollback`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	privs, err := accessRolePrivsFromTF(plan.Privs)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error planning pve_role",
			fmt.Sprintf("reading privs for role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	if err := r.client.CreateAccessRole(ctx, plan.RoleID.ValueString(), privs); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_role",
			fmt.Sprintf("creating role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_role after create",
			fmt.Sprintf("reading role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_role",
			fmt.Sprintf("reading role %s: %s", state.RoleID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	privs, err := accessRolePrivsFromTF(plan.Privs)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error planning pve_role",
			fmt.Sprintf("reading privs for role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	if err := r.client.UpdateAccessRole(ctx, plan.RoleID.ValueString(), privs); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_role",
			fmt.Sprintf("updating role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_role after update",
			fmt.Sprintf("reading role %s: %s", plan.RoleID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccessRole(ctx, state.RoleID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_role",
			fmt.Sprintf("deleting role %s: %s", state.RoleID.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<roleid>`.
func (r *pveRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_role import ID",
			"Expected the import ID to be the role identifier, e.g. `custom-op`.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("roleid"), req.ID)...)
}

// readInto populates the model's computed attributes from PVE.
func (r *pveRoleResource) readInto(ctx context.Context, m *pveRoleResourceModel) error {
	grants, err := r.client.GetAccessRole(ctx, m.RoleID.ValueString())
	if err != nil {
		return err
	}
	granted := make([]string, 0, len(grants))
	for priv, allowed := range grants {
		if allowed {
			granted = append(granted, priv)
		}
	}
	sort.Strings(granted)
	privs, err := accessRolePrivsToTF(granted)
	if err != nil {
		return fmt.Errorf("converting privs of role %s: %w", m.RoleID.ValueString(), err)
	}
	m.Privs = privs
	return nil
}

// accessRolePrivsFromTF flattens the privs set into a []string; a null set
// becomes an empty privilege list (a role without privileges is legal).
func accessRolePrivsFromTF(set types.Set) ([]string, error) {
	if set.IsNull() || set.IsUnknown() {
		return []string{}, nil
	}
	out := make([]string, 0, len(set.Elements()))
	for _, element := range set.Elements() {
		value, ok := element.(types.String)
		if !ok {
			return nil, fmt.Errorf("privilege %v is not a string", element)
		}
		out = append(out, value.ValueString())
	}
	return out, nil
}

// accessRolePrivsToTF builds the privs set from granted privilege names.
// Element types cannot mismatch here, so the diagnostics path is defensive
// only.
func accessRolePrivsToTF(privs []string) (types.Set, error) {
	if privs == nil {
		privs = []string{}
	}
	elements := make([]attr.Value, 0, len(privs))
	for _, priv := range privs {
		elements = append(elements, types.StringValue(priv))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		return types.SetNull(types.StringType), fmt.Errorf("building privs set: %d error(s)", diags.ErrorsCount())
	}
	return set, nil
}
