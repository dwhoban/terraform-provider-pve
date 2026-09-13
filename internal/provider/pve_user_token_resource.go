// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                = &pveUserTokenResource{}
	_ resource.ResourceWithConfigure   = &pveUserTokenResource{}
	_ resource.ResourceWithImportState = &pveUserTokenResource{}
)

// NewPveUserTokenResource returns the resource implementation.
func NewPveUserTokenResource() resource.Resource {
	return &pveUserTokenResource{}
}

// pveUserTokenResource manages a PVE API token via
// /access/users/{userid}/token/{tokenid}.
type pveUserTokenResource struct {
	client *pveclient.Client
}

// pveUserTokenResourceModel is the Terraform-facing shape.
type pveUserTokenResourceModel struct {
	UserID     types.String `tfsdk:"userid"`
	TokenID    types.String `tfsdk:"tokenid"`
	Comment    types.String `tfsdk:"comment"`
	Expire     types.Int64  `tfsdk:"expire"`
	Privsep    types.Bool   `tfsdk:"privsep"`
	TokenValue types.String `tfsdk:"token_value"`
}

// Metadata implements resource.Resource.
func (r *pveUserTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveUserToken
}

// Schema implements resource.Resource.
func (r *pveUserTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a PVE API token for a user account (`/access/users/{userid}/token/{tokenid}`). PVE returns the token secret exactly once, at create; it is stored in the state as `token_value` and never retrievable afterwards. Rotating a secret means replacing the token.",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID in `name@realm` format the token belongs to, e.g. `root@pam`. The user must exist. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tokenid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Token identifier matching `[A-Za-z][A-Za-z0-9.-_]+` (e.g. `ci`). The full token ID is `<userid>!<tokenid>`. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form comment for the token.",
			},
			"expire": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "API token expiration date (seconds since epoch). `0` means no expiration date. An omitted expiry inherits the user's expiry; set `0` to keep the token valid indefinitely. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.Between(0, math.MaxInt64),
				},
			},
			"privsep": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Restrict API token privileges with separate ACLs (default `true`), or give the token the full privileges of the corresponding user.",
			},
			"token_value": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Full API token value in `<userid>!<tokenid>=<secret>` form, returned by PVE only at create time and persisted via `UseStateForUnknown`. Reads never refresh it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveUserTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveUserTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveUserTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateAccessUserToken(ctx,
		plan.UserID.ValueString(), plan.TokenID.ValueString(),
		accessUserTokenBodyFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_user_token",
			fmt.Sprintf("creating token %s for user %s: %s",
				plan.TokenID.ValueString(), plan.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "created PVE user token", map[string]any{
		"userid":  plan.UserID.ValueString(),
		"tokenid": plan.TokenID.ValueString(),
	})
	if err := r.readInto(ctx, &plan); err != nil {
		if isPVEClientNotFound(err) {
			resp.Diagnostics.AddError(
				"Error reading pve_user_token after create",
				fmt.Sprintf("token %s for user %s disappeared immediately after create",
					plan.TokenID.ValueString(), plan.UserID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_user_token after create",
			fmt.Sprintf("reading token %s for user %s: %s",
				plan.TokenID.ValueString(), plan.UserID.ValueString(), err),
		)
		return
	}
	// The one-time secret comes from the create response; reads never return
	// it. UseStateForUnknown keeps it in the state afterwards.
	plan.TokenValue = types.StringValue(created.Value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveUserTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveUserTokenResourceModel
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
			"Error reading pve_user_token",
			fmt.Sprintf("reading token %s for user %s: %s",
				state.TokenID.ValueString(), state.UserID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveUserTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveUserTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Regenerate is deliberately never sent: it would rotate the secret and
	// invalidate the stored token_value.
	if _, err := r.client.UpdateAccessUserToken(ctx,
		plan.UserID.ValueString(), plan.TokenID.ValueString(),
		accessUserTokenBodyFromModel(plan), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_user_token",
			fmt.Sprintf("updating token %s for user %s: %s",
				plan.TokenID.ValueString(), plan.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "updated PVE user token", map[string]any{
		"userid":  plan.UserID.ValueString(),
		"tokenid": plan.TokenID.ValueString(),
	})
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user_token after update",
			fmt.Sprintf("reading token %s for user %s: %s",
				plan.TokenID.ValueString(), plan.UserID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveUserTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveUserTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccessUserToken(ctx, state.UserID.ValueString(), state.TokenID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_user_token",
			fmt.Sprintf("deleting token %s for user %s: %s",
				state.TokenID.ValueString(), state.UserID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE user token", map[string]any{
		"userid":  state.UserID.ValueString(),
		"tokenid": state.TokenID.ValueString(),
	})
}

// ImportState parses an import ID of the form `<userid>!<tokenid>`
// (e.g. `root@pam!ci`).
func (r *pveUserTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "!", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_user_token import ID",
			fmt.Sprintf("expected import ID of the form <userid>!<tokenid> (e.g. root@pam!ci), got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("userid"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tokenid"), parts[1])...)
}

// readInto refreshes every modeled field from the PVE API. The token secret
// is never returned by reads and is handled by the callers.
func (r *pveUserTokenResource) readInto(ctx context.Context, m *pveUserTokenResourceModel) error {
	tok, err := r.client.GetAccessUserToken(ctx, m.UserID.ValueString(), m.TokenID.ValueString())
	if err != nil {
		return err
	}
	m.Comment, m.Expire, m.Privsep = accessUserTokenFieldsToTF(tok.Comment, tok.Expire, tok.Privsep)
	return nil
}

// accessUserTokenBodyFromModel projects the Terraform model into the wire
// body for create and update. Null fields are omitted (on update PVE keeps
// the stored value; an empty comment sent as the empty string clears it).
func accessUserTokenBodyFromModel(m pveUserTokenResourceModel) pveclient.AccessToken {
	body := pveclient.AccessToken{
		Comment: m.Comment.ValueString(),
	}
	if !m.Expire.IsNull() && !m.Expire.IsUnknown() {
		expire := m.Expire.ValueInt64()
		body.Expire = &expire
	}
	if !m.Privsep.IsNull() && !m.Privsep.IsUnknown() {
		privsep := m.Privsep.ValueBool()
		body.Privsep = &privsep
	}
	return body
}

// accessUserTokenFieldsToTF converts wire token fields to Terraform values.
// An empty comment becomes null; an absent expiry stays null (it inherits the
// user's expiry); an absent privsep falls back to PVE's default of `true`.
func accessUserTokenFieldsToTF(comment string, expire *int64, privsep *bool) (types.String, types.Int64, types.Bool) {
	c := types.StringNull()
	if comment != "" {
		c = types.StringValue(comment)
	}
	e := types.Int64Null()
	if expire != nil {
		e = types.Int64Value(*expire)
	}
	p := types.BoolValue(true)
	if privsep != nil {
		p = types.BoolValue(*privsep)
	}
	return c, e, p
}
