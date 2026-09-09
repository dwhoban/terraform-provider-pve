// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// acmeDNSPluginAPIs enumerates the pin's closed `api` enum of supported DNS
// API plugin names.
var acmeDNSPluginAPIs = []string{
	"1984hosting", "acmedns", "acmeproxy", "active24", "ad", "ali",
	"alviy", "anx", "artfiles", "arvan", "aurora", "autodns",
	"aws", "azion", "azure", "beget", "bookmyname", "bunny",
	"cf", "clouddns", "cloudns", "cn", "conoha", "constellix",
	"cpanel", "curanet", "cyon", "da", "ddnss", "desec",
	"df", "dgon", "dnsexit", "dnshome", "dnsimple", "dnsservices",
	"doapi", "domeneshop", "dp", "dpi", "dreamhost", "duckdns",
	"durabledns", "dyn", "dynu", "dynv6", "easydns", "edgecenter",
	"edgedns", "euserv", "exoscale", "fornex", "freedns", "freemyip",
	"gandi_livedns", "gcloud", "gcore", "gd", "geoscaling", "googledomains",
	"he", "he_ddns", "hetzner", "hetznercloud", "hexonet", "hostingde",
	"huaweicloud", "infoblox", "infomaniak", "internetbs", "inwx", //nolint:misspell // internet.bs is an acme.sh provider name., "ionos",
	"ionos_cloud", "ipv64", "ispconfig", "jd", "joker", "kappernet",
	"kas", "kinghost", "knot", "la", "leaseweb", "lexicon",
	"limacity", "linode", "linode_v4", "loopia", "lua", "maradns",
	"me", "miab", "mijnhost", "misaka", "myapi", "mydevil",
	"mydnsjp", "mythic_beasts", "namecheap", "namecom", "namesilo", "nanelo",
	"nederhost", "neodigit", "netcup", "netlify", "nic", "njalla",
	"nm", "nsd", "nsone", "nsupdate", "nw", "oci",
	"omglol", "one", "online", "openprovider", "openprovider_rest", "openstack",
	"opnsense", "ovh", "pdns", "pleskxml", "pointhq", "porkbun",
	"rackcorp", "rackspace", "rage4", "rcode0", "regru", "scaleway",
	"schlundtech", "selectel", "selfhost", "servercow", "simply", "spaceship",
	"technitium", "tele3", "tencent", "timeweb", "transip", "udr",
	"ultra", "unoeuro", "variomedia", "veesp", "vercel", "vscale",
	"vultr", "websupport", "west_cn", "world4you", "yandex360", "yc",
	"zilore", "zone", "zoneedit", "zonomi",
}

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveAcmeDnsPluginResource{}
	_ resource.ResourceWithConfigure   = &pveAcmeDnsPluginResource{}
	_ resource.ResourceWithImportState = &pveAcmeDnsPluginResource{}
)

// NewPveAcmeDnsPluginResource returns the resource implementation.
func NewPveAcmeDnsPluginResource() resource.Resource {
	return &pveAcmeDnsPluginResource{}
}

// pveAcmeDnsPluginResource manages a DNS ACME challenge plugin
// (/cluster/acme/plugins with the pin's challenge type fixed to `dns`).
// Every mutation is synchronous per the pin.
type pveAcmeDnsPluginResource struct {
	client *pveclient.Client
}

// pveAcmeDnsPluginResourceModel is the Terraform-facing shape.
type pveAcmeDnsPluginResourceModel struct {
	Plugin          types.String `tfsdk:"plugin"`
	Type            types.String `tfsdk:"type"`
	API             types.String `tfsdk:"api"`
	Data            types.String `tfsdk:"data"`
	Disable         types.Bool   `tfsdk:"disable"`
	Nodes           types.List   `tfsdk:"nodes"`
	ValidationDelay types.Int64  `tfsdk:"validation_delay"`
	Digest          types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeDnsPlugin
}

// Schema implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DNS ACME challenge plugin (`/cluster/acme/plugins` with the challenge type fixed to `dns`). The credentials in `data` are base64-encoded by you — PVE stores the value verbatim. Mutations are synchronous per the pin.",
		Attributes: map[string]schema.Attribute{
			"plugin": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique identifier for the ACME plugin instance (PVE `pve-configid` format; the pin names the create parameter `id` and the response key `plugin`). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ACME challenge type; always `dns` for this resource.",
			},
			"api": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "DNS API plugin name. Must be one of: `" + strings.Join(acmeDNSPluginAPIs, "`, `") + "`.",
				Validators: []validator.String{
					stringvalidator.OneOf(acmeDNSPluginAPIs...),
				},
			},
			"data": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "DNS plugin data with your API credentials for the chosen `api` (base64 encoded, e.g. `base64 -w0 < creds-file`).",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Flag to disable the plugin config.",
			},
			"nodes": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Cluster nodes the plugin is limited to (PVE `pve-node-list`); empty means all nodes.",
			},
			"validation_delay": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Extra delay in seconds to wait before requesting validation, to cope with a long DNS record TTL. Must be between 0 and 172800. PVE defaults to 30.",
				Validators: []validator.Int64{
					int64validator.Between(0, 172800),
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the plugin configuration file; used for concurrent-modification detection.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveAcmeDnsPluginResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveAcmeDnsPluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_acme_dns_plugin", "provider client is not configured")
		return
	}
	if err := r.client.CreateAcmePlugin(ctx, acmeDnsPluginFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_acme_dns_plugin",
			fmt.Sprintf("creating ACME dns plugin %s: %s", plan.Plugin.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_dns_plugin after create",
			fmt.Sprintf("reading ACME dns plugin %s: %s", plan.Plugin.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveAcmeDnsPluginResourceModel
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
			"Error reading pve_acme_dns_plugin",
			fmt.Sprintf("reading ACME dns plugin %s: %s", state.Plugin.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveAcmeDnsPluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveAcmeDnsPluginResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_acme_dns_plugin", "provider client is not configured")
		return
	}
	deleteFields := acmeDnsPluginDeleteFields(plan, state)
	if err := r.client.UpdateAcmePlugin(ctx, plan.Plugin.ValueString(), acmeDnsPluginFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_acme_dns_plugin",
			fmt.Sprintf("updating ACME dns plugin %s: %s", plan.Plugin.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_dns_plugin after update",
			fmt.Sprintf("reading ACME dns plugin %s: %s", plan.Plugin.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveAcmeDnsPluginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveAcmeDnsPluginResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAcmePlugin(ctx, state.Plugin.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_acme_dns_plugin",
			fmt.Sprintf("deleting ACME dns plugin %s: %s", state.Plugin.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<plugin>`.
func (r *pveAcmeDnsPluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_acme_dns_plugin import ID", "import ID must be the ACME plugin identifier, e.g. `pdns`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plugin"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveAcmeDnsPluginResource) readInto(ctx context.Context, m *pveAcmeDnsPluginResourceModel) error {
	plugin, err := r.client.GetAcmePlugin(ctx, m.Plugin.ValueString())
	if err != nil {
		return err
	}
	m.Type = types.StringValue(plugin.Type)
	m.API = nodeNetworkStringToTF(plugin.API)
	m.Data = nodeNetworkStringToTF(plugin.Data)
	m.Disable = nodeNetworkBoolPtrToTF(plugin.Disable)
	m.Nodes = listStringToTF(plugin.Nodes)
	m.ValidationDelay = haInt64PtrToTF(plugin.ValidationDelay)
	m.Digest = nodeNetworkStringToTF(plugin.Digest)
	return nil
}

// acmeDnsPluginFromModel projects the Terraform model into the wire body,
// fixing the pin's challenge type to `dns`.
func acmeDnsPluginFromModel(m pveAcmeDnsPluginResourceModel) pveclient.AcmePlugin {
	body := pveclient.AcmePlugin{
		Plugin: m.Plugin.ValueString(),
		Type:   "dns",
		Nodes:  listStringFromTF(m.Nodes),
	}
	if !m.API.IsNull() && !m.API.IsUnknown() {
		body.API = m.API.ValueString()
	}
	if !m.Data.IsNull() && !m.Data.IsUnknown() {
		body.Data = m.Data.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		body.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	if !m.ValidationDelay.IsNull() && !m.ValidationDelay.IsUnknown() {
		body.ValidationDelay = pveclient.HAInt64Ptr(m.ValidationDelay.ValueInt64())
	}
	return body
}

// acmeDnsPluginDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func acmeDnsPluginDeleteFields(plan, state pveAcmeDnsPluginResourceModel) []string {
	var out []string
	if plan.API.IsNull() && !state.API.IsNull() {
		out = append(out, "api")
	}
	if plan.Data.IsNull() && !state.Data.IsNull() {
		out = append(out, "data")
	}
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	if plan.Nodes.IsNull() && !state.Nodes.IsNull() {
		out = append(out, "nodes")
	}
	if plan.ValidationDelay.IsNull() && !state.ValidationDelay.IsNull() {
		out = append(out, "validation-delay")
	}
	return out
}
