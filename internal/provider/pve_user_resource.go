// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveUserResource{}
	_ resource.ResourceWithConfigure   = &pveUserResource{}
	_ resource.ResourceWithImportState = &pveUserResource{}
)

// NewPveUserResource returns the resource implementation.
func NewPveUserResource() resource.Resource {
	return &pveUserResource{}
}

// pveUserResource manages a PVE user account via /access/users/{userid}.
type pveUserResource struct {
	client *pveclient.Client
}

// pveUserResourceModel is the Terraform-facing shape.
type pveUserResourceModel struct {
	UserID    types.String `tfsdk:"userid"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Keys      types.String `tfsdk:"keys"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Groups    types.Set    `tfsdk:"groups"`
	Password  types.String `tfsdk:"password"`
}

// Metadata implements resource.Resource.
func (r *pveUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveUser
}

// Schema implements resource.Resource.
func (r *pveUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a PVE user account (`/access/users/{userid}`). The account lives in the realm encoded in `userid`; the realm itself must already exist. Password changes after create go through PVE's separate password endpoint, which this resource does not model.",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID in `name@realm` format, e.g. `root@pam` or `ci@pve`. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form comment, up to 2048 characters.",
			},
			"email": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Email address, up to 254 characters.",
			},
			"firstname": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "First name, up to 1024 characters.",
			},
			"lastname": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Last name, up to 1024 characters.",
			},
			"keys": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Keys for two factor auth (yubico): up to 4096 characters of `[0-9a-zA-Z!=]`.",
			},
			"enable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Enable the account. Defaults to `true`; set `false` to disable login.",
			},
			"expire": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Account expiration date (seconds since epoch). `0` means no expiration date. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.Between(0, math.MaxInt64),
				},
			},
			"groups": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Set of PVE group IDs the user belongs to. The groups themselves are managed by `pve_group`; removing every entry here removes the user from all groups.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Initial password (8 to 64 characters). Only sent at create; PVE manages later password changes through its own password endpoint, so changes to this attribute after create have no effect.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := accessUserBodyFromModel(plan)
	if err := r.client.CreateAccessUser(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_user",
			fmt.Sprintf("creating user %s: %s", plan.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "created PVE user", map[string]any{"userid": plan.UserID.ValueString()})
	if err := r.readInto(ctx, &plan); err != nil {
		if isPVEClientNotFound(err) {
			resp.Diagnostics.AddError(
				"Error reading pve_user after create",
				fmt.Sprintf("user %s disappeared immediately after create", plan.UserID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_user after create",
			fmt.Sprintf("reading user %s: %s", plan.UserID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveUserResourceModel
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
			"Error reading pve_user",
			fmt.Sprintf("reading user %s: %s", state.UserID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// PVE's update_user endpoint has no `delete` parameter, so the body
	// always carries the full field set and cleared attributes travel as
	// empty values (empty string / empty group list).
	body := accessUserBodyFromModel(plan)
	if err := r.client.UpdateAccessUser(ctx, plan.UserID.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_user",
			fmt.Sprintf("updating user %s: %s", plan.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "updated PVE user", map[string]any{"userid": plan.UserID.ValueString()})
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user after update",
			fmt.Sprintf("reading user %s: %s", plan.UserID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccessUser(ctx, state.UserID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_user",
			fmt.Sprintf("deleting user %s: %s", state.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE user", map[string]any{"userid": state.UserID.ValueString()})
}

// ImportState parses an import ID of the form `<userid>` (e.g. `root@pam`).
func (r *pveUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("userid"), req, resp)
}

// readInto refreshes every modeled field from the PVE API. The password is
// never returned by the API and stays whatever the configuration carried.
func (r *pveUserResource) readInto(ctx context.Context, m *pveUserResourceModel) error {
	user, err := r.client.GetAccessUser(ctx, m.UserID.ValueString())
	if err != nil {
		return err
	}
	m.Comment = accessUserStringToTF(user.Comment)
	m.Email = accessUserStringToTF(user.Email)
	m.Firstname = accessUserStringToTF(user.Firstname)
	m.Lastname = accessUserStringToTF(user.Lastname)
	m.Keys = accessUserStringToTF(user.Keys)
	// Absent enable means PVE's default: the account is enabled.
	m.Enable = types.BoolValue(user.Enable == nil || *user.Enable)
	// Absent expire means PVE's default: no expiration date.
	m.Expire = types.Int64Value(0)
	if user.Expire != nil {
		m.Expire = types.Int64Value(*user.Expire)
	}
	groups, gerr := accessUserGroupsToTF(user.Groups)
	if gerr != nil {
		return fmt.Errorf("building groups set for user %s: %w", m.UserID.ValueString(), gerr)
	}
	m.Groups = groups
	return nil
}

// accessUserBodyFromModel projects the Terraform model into the wire body.
// Null strings become empty values (PVE clears them on update and ignores
// them on create); a null enable becomes PVE's default of enabled.
func accessUserBodyFromModel(m pveUserResourceModel) pveclient.AccessUser {
	body := pveclient.AccessUser{
		UserID:    m.UserID.ValueString(),
		Comment:   m.Comment.ValueString(),
		Email:     m.Email.ValueString(),
		Firstname: m.Firstname.ValueString(),
		Lastname:  m.Lastname.ValueString(),
		Keys:      m.Keys.ValueString(),
	}
	enable := true
	if !m.Enable.IsNull() && !m.Enable.IsUnknown() {
		enable = m.Enable.ValueBool()
	}
	body.Enable = &enable
	if !m.Expire.IsNull() && !m.Expire.IsUnknown() {
		expire := m.Expire.ValueInt64()
		body.Expire = &expire
	}
	if !m.Password.IsNull() && !m.Password.IsUnknown() {
		body.Password = m.Password.ValueString()
	}
	return body
}

// accessUserGroupsToTF builds the groups set; an absent or empty membership
// list becomes a null set so optional groups do not churn between plans.
func accessUserGroupsToTF(groups []string) (types.Set, error) {
	if len(groups) == 0 {
		return types.SetNull(types.StringType), nil
	}
	elements := make([]attr.Value, 0, len(groups))
	for _, g := range groups {
		elements = append(elements, types.StringValue(g))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		return types.SetNull(types.StringType), fmt.Errorf("building groups set: %d error(s)", diags.ErrorsCount())
	}
	return set, nil
}

// accessUserStringToTF maps the empty string to null so cleared optional
// fields do not churn between reads and plans.
func accessUserStringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
