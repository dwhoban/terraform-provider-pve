// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

var (
	_ resource.Resource                   = &pveAclResource{}
	_ resource.ResourceWithConfigure      = &pveAclResource{}
	_ resource.ResourceWithImportState    = &pveAclResource{}
	_ resource.ResourceWithValidateConfig = &pveAclResource{}
)

// NewPveAclResource returns the resource implementation.
func NewPveAclResource() resource.Resource {
	return &pveAclResource{}
}

// ACL subject types accepted by PUT /access/acl.
const (
	aclTypeUser  = "user"
	aclTypeGroup = "group"
	aclTypeToken = "token"
)

// errAclEntryNotFound reports that the managed entry is absent from
// GET /access/acl; Read maps it to removal from state.
var errAclEntryNotFound = errors.New("ACL entry not found")

// pveAclResource manages a single access control list entry via
// GET/PUT /access/acl. The pin defines no DELETE verb; removal is a PUT
// with the delete flag. All mutations are synchronous.
type pveAclResource struct {
	client *pveclient.Client
}

// pveAclResourceModel is the Terraform-facing shape.
type pveAclResourceModel struct {
	Path      types.String `tfsdk:"path"`
	Role      types.String `tfsdk:"role"`
	Type      types.String `tfsdk:"type"`
	UserID    types.String `tfsdk:"user_id"`
	GroupID   types.String `tfsdk:"group_id"`
	TokenID   types.String `tfsdk:"token_id"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

// Metadata implements resource.Resource.
func (r *pveAclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcl
}

// Schema implements resource.Resource.
func (r *pveAclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single access control list entry on the Proxmox VE cluster (`GET/PUT /access/acl`). Changing `path`, `role`, or `type` replaces the entry; deletion issues a PUT with the `delete` flag because the API defines no DELETE verb on this path.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Access control path the entry applies to, e.g. `/`, `/vms/100`, or `/storage/local`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"role": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role name granted on `path`, e.g. `Administrator` or `PVEVMUser`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Kind of subject the entry grants to. Must be one of: `user`, `group`, `token`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "User ID (`user@realm`) when `type` is `user`. Exactly one of `user_id`, `group_id`, or `token_id` must be set, matching `type`.",
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Group ID when `type` is `group`. Exactly one of `user_id`, `group_id`, or `token_id` must be set, matching `type`.",
			},
			"token_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Full API token ID (`user@realm!tokenname`) when `type` is `token`. Exactly one of `user_id`, `group_id`, or `token_id` must be set, matching `type`.",
			},
			"propagate": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Allow the permission to propagate (inherit) to child paths. Defaults to `true` (the PVE server default).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveAclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = aclConfigureResource(req, resp)
}

// ValidateConfig implements resource.ResourceWithValidateConfig. It enforces
// that exactly one identity attribute is set and that it matches `type`.
func (r *pveAclResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m pveAclResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Type.IsNull() || m.Type.IsUnknown() || m.UserID.IsUnknown() || m.GroupID.IsUnknown() || m.TokenID.IsUnknown() {
		return
	}
	set := []string{aclTypeUser, aclTypeGroup, aclTypeToken}
	count := 0
	for _, subject := range set {
		if !aclIdentityValue(m, subject).IsNull() {
			count++
		}
	}
	want := aclIdentityAttr(m.Type.ValueString())
	if count != 1 {
		resp.Diagnostics.AddAttributeError(
			path.Root(want),
			"Invalid pve_acl identity",
			fmt.Sprintf("exactly one of user_id, group_id, or token_id must be set, got %d.", count),
		)
		return
	}
	if aclIdentityValue(m, m.Type.ValueString()).IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root(want),
			"Invalid pve_acl identity",
			fmt.Sprintf("type is %q but the matching %s attribute is not set.", m.Type.ValueString(), want),
		)
	}
}

// Create implements resource.Resource.
func (r *pveAclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveAclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutAcl(ctx, aclUpdateFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_acl",
			fmt.Sprintf("adding ACL entry %s on %s: %s", aclDescribe(plan), plan.Path.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acl after create",
			fmt.Sprintf("reading ACL entry %s: %s", aclDescribe(plan), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveAclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveAclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if errors.Is(err, errAclEntryNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_acl",
			fmt.Sprintf("reading ACL entry %s: %s", aclDescribe(state), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. The identity attributes force
// replacement, so only `propagate` can change here; PUT /access/acl is
// idempotent and replaces the entry wholesale.
func (r *pveAclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveAclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutAcl(ctx, aclUpdateFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_acl",
			fmt.Sprintf("updating ACL entry %s: %s", aclDescribe(plan), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acl after update",
			fmt.Sprintf("reading ACL entry %s: %s", aclDescribe(plan), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Removing an already-absent entry is
// a server-side no-op and therefore success.
func (r *pveAclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveAclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAcl(ctx, aclEntryFromModel(state)); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting pve_acl",
			fmt.Sprintf("deleting ACL entry %s: %s", aclDescribe(state), err),
		)
	}
}

// ImportState parses an import ID of the form `<path>|<role>|<type>|<id>`,
// e.g. `/vms/100|PVEVMUser|user|ops@pam`.
func (r *pveAclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pathPart, role, typ, ugid, err := aclParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_acl import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), pathPart)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), role)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), typ)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(aclIdentityAttr(typ)), ugid)...)
}

// readInto refreshes the model from GET /access/acl, locating the managed
// entry by its full identity. errAclEntryNotFound signals removal.
func (r *pveAclResource) readInto(ctx context.Context, m *pveAclResourceModel) error {
	entries, err := r.client.GetAcl(ctx)
	if err != nil {
		return fmt.Errorf("listing ACLs: %w", err)
	}
	want := aclEntryFromModel(*m)
	for _, entry := range entries {
		if entry.Path == want.Path && entry.RoleID == want.RoleID && entry.Type == want.Type && entry.Ugid == want.Ugid {
			aclEntryIntoModel(m, entry)
			return nil
		}
	}
	return errAclEntryNotFound
}

// aclConfigureResource extracts the configured *pveclient.Client from a
// resource ConfigureRequest. Nil provider data leaves the resource
// unconfigured (unit tests).
func aclConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
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

// aclIdentityAttr maps a subject type to its Terraform attribute name.
func aclIdentityAttr(subject string) string {
	switch subject {
	case aclTypeGroup:
		return "group_id"
	case aclTypeToken:
		return "token_id"
	default:
		return "user_id"
	}
}

// aclIdentityValue returns the identity attribute matching subject.
func aclIdentityValue(m pveAclResourceModel, subject string) types.String {
	switch subject {
	case aclTypeGroup:
		return m.GroupID
	case aclTypeToken:
		return m.TokenID
	default:
		return m.UserID
	}
}

// aclUgid returns the identity value matching the model's type.
func aclUgid(m pveAclResourceModel) string {
	return aclIdentityValue(m, m.Type.ValueString()).ValueString()
}

// aclUpdateFromModel projects the model into a PUT /access/acl body. The
// body carries exactly one identity list parameter, matching the entry
// type; removal is handled by DeleteAcl at the client layer.
func aclUpdateFromModel(m pveAclResourceModel) pveclient.AclUpdate {
	update := pveclient.AclUpdate{
		Path:  m.Path.ValueString(),
		Roles: m.Role.ValueString(),
	}
	if !m.Propagate.IsNull() && !m.Propagate.IsUnknown() {
		v := m.Propagate.ValueBool()
		update.Propagate = &v
	}
	switch m.Type.ValueString() {
	case aclTypeGroup:
		update.Groups = aclUgid(m)
	case aclTypeToken:
		update.Tokens = aclUgid(m)
	default:
		update.Users = aclUgid(m)
	}
	return update
}

// aclEntryFromModel projects the model into the wire entry shape.
func aclEntryFromModel(m pveAclResourceModel) pveclient.AclEntry {
	return pveclient.AclEntry{
		Path:   m.Path.ValueString(),
		RoleID: m.Role.ValueString(),
		Type:   m.Type.ValueString(),
		Ugid:   aclUgid(m),
	}
}

// aclEntryIntoModel copies a wire entry into the model. An absent propagate
// flag means the PVE default (true).
func aclEntryIntoModel(m *pveAclResourceModel, entry pveclient.AclEntry) {
	m.Propagate = types.BoolValue(entry.Propagate == nil || *entry.Propagate)
}

// aclDescribe renders the entry identity for diagnostics. ACL entries carry
// no secret material.
func aclDescribe(m pveAclResourceModel) string {
	return fmt.Sprintf("%s for %s on %s", m.Role.ValueString(), aclUgid(m), m.Path.ValueString())
}

// aclParseImportID splits `<path>|<role>|<type>|<id>` into its parts.
func aclParseImportID(id string) (string, string, string, string, error) {
	parts := strings.Split(id, "|")
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("expected 4 `|`-separated parts (path|role|type|id), got %d", len(parts))
	}
	for i, part := range parts {
		if part == "" {
			return "", "", "", "", fmt.Errorf("part %d of the import ID is empty", i+1)
		}
	}
	if parts[2] != aclTypeUser && parts[2] != aclTypeGroup && parts[2] != aclTypeToken {
		return "", "", "", "", fmt.Errorf("type %q must be one of: %s, %s, %s", parts[2], aclTypeUser, aclTypeGroup, aclTypeToken)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}
