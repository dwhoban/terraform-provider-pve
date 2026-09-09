// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnDnsResource{}
	_ resource.ResourceWithConfigure   = &pveSdnDnsResource{}
	_ resource.ResourceWithImportState = &pveSdnDnsResource{}
)

// NewPveSdnDnsResource returns the resource implementation.
func NewPveSdnDnsResource() resource.Resource {
	return &pveSdnDnsResource{}
}

// pveSdnDnsResource manages an SDN reverse-DNS plugin object
// (/cluster/sdn/dns). The pin's only plugin type is powerdns; every
// mutation is synchronous.
type pveSdnDnsResource struct {
	client *pveclient.Client
}

// pveSdnDnsResourceModel is the Terraform-facing shape.
type pveSdnDnsResourceModel struct {
	Dns           types.String `tfsdk:"dns"`
	Type          types.String `tfsdk:"type"`
	Key           types.String `tfsdk:"key"`
	URL           types.String `tfsdk:"url"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Reversemaskv6 types.Int64  `tfsdk:"reversemaskv6"`
	TTL           types.Int64  `tfsdk:"ttl"`
}

// Metadata implements resource.Resource.
func (r *pveSdnDnsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnDns
}

// Schema implements resource.Resource.
func (r *pveSdnDnsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SDN reverse-DNS plugin object (`/cluster/sdn/dns`). The pin's only plugin type is `powerdns`. Changes stay pending until the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"dns": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN DNS object identifier (2 or more characters, starting with a letter). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS plugin type; always `powerdns` for this resource.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "API authentication key for the PowerDNS server.",
			},
			"url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "URL of the PowerDNS API server (for example `https://pdns.example:8081/api/v1`).",
			},
			"fingerprint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Certificate SHA 256 fingerprint of the PowerDNS server, as 32 colon-separated hex pairs.",
			},
			"reversemaskv6": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Mask for IPv6 reverse lookups.",
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Record TTL for registered entries.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnDnsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnDnsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnDnsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_dns", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnDns(ctx, sdnDnsFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_dns",
			fmt.Sprintf("creating SDN dns %s: %s", plan.Dns.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_dns after create",
			fmt.Sprintf("reading SDN dns %s: %s", plan.Dns.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnDnsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnDnsResourceModel
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
			"Error reading pve_sdn_dns",
			fmt.Sprintf("reading SDN dns %s: %s", state.Dns.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnDnsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnDnsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnDnsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_sdn_dns", "provider client is not configured")
		return
	}
	deleteFields := sdnDnsDeleteFields(plan, state)
	if err := r.client.UpdateSdnDns(ctx, plan.Dns.ValueString(), sdnDnsFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_dns",
			fmt.Sprintf("updating SDN dns %s: %s", plan.Dns.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_dns after update",
			fmt.Sprintf("reading SDN dns %s: %s", plan.Dns.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnDnsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnDnsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_sdn_dns", "provider client is not configured")
		return
	}
	if err := r.client.DeleteSdnDns(ctx, state.Dns.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_dns",
			fmt.Sprintf("deleting SDN dns %s: %s", state.Dns.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<dns>`.
func (r *pveSdnDnsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_dns import ID", "import ID must be the DNS plugin identifier, e.g. `pdns1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dns"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveSdnDnsResource) readInto(ctx context.Context, m *pveSdnDnsResourceModel) error {
	dns, err := r.client.GetSdnDns(ctx, m.Dns.ValueString())
	if err != nil {
		return err
	}
	m.Type = nodeNetworkStringToTF(dns.Type)
	m.Key = nodeNetworkStringToTF(dns.Key)
	m.URL = nodeNetworkStringToTF(dns.URL)
	m.Fingerprint = nodeNetworkStringToTF(dns.Fingerprint)
	m.Reversemaskv6 = haInt64PtrToTF(dns.Reversemaskv6)
	m.TTL = haInt64PtrToTF(dns.TTL)
	return nil
}

// sdnDnsFromModel projects the Terraform model into the wire body.
func sdnDnsFromModel(m pveSdnDnsResourceModel) pveclient.SdnDns {
	body := pveclient.SdnDns{
		Dns: m.Dns.ValueString(),
		Key: m.Key.ValueString(),
		URL: m.URL.ValueString(),
	}
	if !m.Fingerprint.IsNull() && !m.Fingerprint.IsUnknown() {
		body.Fingerprint = m.Fingerprint.ValueString()
	}
	if !m.Reversemaskv6.IsNull() && !m.Reversemaskv6.IsUnknown() {
		body.Reversemaskv6 = pveclient.HAInt64Ptr(m.Reversemaskv6.ValueInt64())
	}
	if !m.TTL.IsNull() && !m.TTL.IsUnknown() {
		body.TTL = pveclient.HAInt64Ptr(m.TTL.ValueInt64())
	}
	return body
}

// sdnDnsDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func sdnDnsDeleteFields(plan, state pveSdnDnsResourceModel) []string {
	var out []string
	if plan.Fingerprint.IsNull() && !state.Fingerprint.IsNull() {
		out = append(out, "fingerprint")
	}
	if plan.Reversemaskv6.IsNull() && !state.Reversemaskv6.IsNull() {
		out = append(out, "reversemaskv6")
	}
	if plan.TTL.IsNull() && !state.TTL.IsNull() {
		out = append(out, "ttl")
	}
	return out
}
