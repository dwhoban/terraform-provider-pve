// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSourceWithConfigure = &pveUserTokenDataSource{}
	_ datasource.DataSource              = &pveUserTokenDataSource{}
)

// NewPveUserTokenDataSource returns the data source implementation.
func NewPveUserTokenDataSource() datasource.DataSource {
	return &pveUserTokenDataSource{}
}

// pveUserTokenDataSource reads one API token of a user or lists all of the
// user's tokens via /access/users/{userid}/token[/{tokenid}].
type pveUserTokenDataSource struct {
	client *pveclient.Client
}

// pveUserTokenDataSourceModel is the Terraform-facing shape.
type pveUserTokenDataSourceModel struct {
	UserID  types.String                       `tfsdk:"userid"`
	TokenID types.String                       `tfsdk:"tokenid"`
	Comment types.String                       `tfsdk:"comment"`
	Expire  types.Int64                        `tfsdk:"expire"`
	Privsep types.Bool                         `tfsdk:"privsep"`
	Tokens  []pveUserTokenDataSourceTokenModel `tfsdk:"tokens"`
	ID      types.String                       `tfsdk:"id"`
}

// pveUserTokenDataSourceTokenModel mirrors the per-row schema of the token
// list.
type pveUserTokenDataSourceTokenModel struct {
	TokenID types.String `tfsdk:"tokenid"`
	Comment types.String `tfsdk:"comment"`
	Expire  types.Int64  `tfsdk:"expire"`
	Privsep types.Bool   `tfsdk:"privsep"`
}

// Metadata implements datasource.DataSource.
func (d *pveUserTokenDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveUserToken
}

// Schema implements datasource.DataSource.
func (d *pveUserTokenDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads API tokens of a PVE user (`GET /access/users/{userid}/token[/{tokenid}]`). Set `tokenid` to read a single token's settings; omit it to list every token of the user. The token secret is never returned by reads, so `token_value` is exclusive to the `pve_user_token` resource at create time.",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID in `name@realm` format whose tokens to read, e.g. `root@pam`.",
			},
			"tokenid": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Token identifier to read a single token. Omit to list all tokens in `tokens` instead.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-form comment of the token (single mode only).",
			},
			"expire": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "API token expiration date (seconds since epoch). `0` means no expiration date; null inherits the user's expiry (single mode only).",
			},
			"privsep": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the token's privileges are restricted with separate ACLs (single mode only).",
			},
			"tokens": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The user's tokens (list mode only, when `tokenid` is omitted).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"tokenid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Token identifier.",
						},
						"comment": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Free-form comment.",
						},
						"expire": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Expiration date (seconds since epoch). `0` means no expiration date; null inherits the user's expiry.",
						},
						"privsep": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the token's privileges are restricted with separate ACLs.",
						},
					},
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`<userid>!<tokenid>` in single mode, `<userid>` in list mode.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveUserTokenDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveUserTokenDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveUserTokenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.TokenID.IsNull() {
		d.readSingle(ctx, &data, resp)
		return
	}
	d.readList(ctx, &data, resp)
}

// readSingle populates the scalar fields from GET .../token/{tokenid}.
func (d *pveUserTokenDataSource) readSingle(ctx context.Context, data *pveUserTokenDataSourceModel, resp *datasource.ReadResponse) {
	tok, err := d.client.GetAccessUserToken(ctx, data.UserID.ValueString(), data.TokenID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user_token data source",
			fmt.Sprintf("reading token %s for user %s: %s",
				data.TokenID.ValueString(), data.UserID.ValueString(), err),
		)
		return
	}
	data.Comment, data.Expire, data.Privsep = accessUserTokenFieldsToTF(tok.Comment, tok.Expire, tok.Privsep)
	data.Tokens = nil
	data.ID = types.StringValue(data.UserID.ValueString() + "!" + data.TokenID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// readList populates the tokens list from GET .../token.
func (d *pveUserTokenDataSource) readList(ctx context.Context, data *pveUserTokenDataSourceModel, resp *datasource.ReadResponse) {
	tokens, err := d.client.ListAccessUserTokens(ctx, data.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user_token data source",
			fmt.Sprintf("listing tokens of user %s: %s", data.UserID.ValueString(), err),
		)
		return
	}
	rows := make([]pveUserTokenDataSourceTokenModel, 0, len(tokens))
	for _, tok := range tokens {
		row := pveUserTokenDataSourceTokenModel{TokenID: types.StringValue(tok.TokenID)}
		row.Comment, row.Expire, row.Privsep = accessUserTokenFieldsToTF(tok.Comment, tok.Expire, tok.Privsep)
		rows = append(rows, row)
	}
	data.Tokens = rows
	data.ID = data.UserID
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}
