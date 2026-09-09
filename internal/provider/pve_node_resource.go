// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeResource{}
	_ resource.ResourceWithConfigure   = &pveNodeResource{}
	_ resource.ResourceWithImportState = &pveNodeResource{}
)

// NewPveNodeResource returns the resource implementation as a framework type.
func NewPveNodeResource() resource.Resource {
	return &pveNodeResource{}
}

// pveNodeResource models the node-level configuration: PVE /nodes/{node}/config
// plus the bundled /dns and /time endpoints.
type pveNodeResource struct {
	client *pveclient.Client
}

// pveNodeResourceModel is the Terraform-facing shape of pve_node.
type pveNodeResourceModel struct {
	Node                types.String             `tfsdk:"node"`
	Description         types.String             `tfsdk:"description"`
	WakeOnLAN           types.String             `tfsdk:"wakeonlan"`
	StartAllOnBootDelay types.Int64              `tfsdk:"startall_onboot_delay"`
	ACMEDomains         []pveNodeACMEDomainModel `tfsdk:"acme_domain"`
	DNSSearch           types.String             `tfsdk:"dns_search"`
	DNS1                types.String             `tfsdk:"dns1"`
	DNS2                types.String             `tfsdk:"dns2"`
	DNS3                types.String             `tfsdk:"dns3"`
	Timezone            types.String             `tfsdk:"timezone"`
	Digest              types.String             `tfsdk:"digest"`
}

func (r *pveNodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNode
}

// pveNodeACMEDomainModel is the per-domain block of pve_node.acme_domain.
type pveNodeACMEDomainModel struct {
	Domain types.String   `tfsdk:"domain"`
	Alias  []types.String `tfsdk:"alias"`
}

// macAddressRe matches a single MAC address in the colon-separated form PVE
// accepts for wakeonlan.
var macAddressRe = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// Schema implements resource.Resource.
func (r *pveNodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Proxmox VE node-level configuration (description, wake-on-LAN, ACME registrations, DNS, and timezone).",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The PVE node name (e.g. `pve1`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form description shown in the PVE web UI.",
			},
			"wakeonlan": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "MAC address to send a Wake-on-LAN packet to on cluster start (colon-separated, e.g. `AA:BB:CC:DD:EE:FF`).",
				Validators: []validator.String{
					stringvalidator.RegexMatches(macAddressRe, "must be a MAC address in colon-separated form (aa:bb:cc:dd:ee:ff)"),
				},
			},
			"startall_onboot_delay": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Delay in seconds before auto-starting any VMs on cluster boot.",
			},
			"acme_domain": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "ACME domains to register with the PVE ACME account.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(64),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"domain": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Fully qualified domain name to register. Must be between 1 and 253 characters.",
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, 253),
							},
						},
						"alias": schema.ListAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Optional alias names registered for the same certificate.",
						},
					},
				},
			},
			"dns_search": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Search domain appended to short hostnames for DNS lookups.",
			},
			"dns1": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Primary DNS server.",
			},
			"dns2": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Secondary DNS server.",
			},
			"dns3": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Tertiary DNS server.",
			},
			"timezone": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IANA timezone name (e.g. `Europe/Berlin`). PVE accepts any value `timedatectl list-timezones` accepts; the provider does not maintain a whitelist. Must be between 1 and 64 characters.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 64),
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Opaque digest returned by PVE; populated after each read and passed back on update for optimistic concurrency.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveNodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applyConfig(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating pve_node", fmt.Sprintf("creating pve_node (%s): %s", plan.Node.ValueString(), err))
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node after create", fmt.Sprintf("reading pve_node (%s) after create: %s", plan.Node.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_node", fmt.Sprintf("reading pve_node (%s): %s", state.Node.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applyConfig(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating pve_node", fmt.Sprintf("updating pve_node (%s): %s", plan.Node.ValueString(), err))
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node after update", fmt.Sprintf("reading pve_node (%s) after update: %s", plan.Node.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. pve_node models node *configuration*,
// so deleting the resource removes it from Terraform management rather
// than destroying the host.
func (r *pveNodeResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Intentional no-op: PVE nodes are physical/VM hosts and are not
	// destroyed by removing the Terraform resource. The configuration
	// attributes can still drift after removal; users who want a clean
	// state should `terraform state rm` the resource explicitly.
	resp.Diagnostics.AddWarning(
		"pve_node removed from Terraform state",
		"pve_node represents the configuration of a Proxmox VE node, not the node itself. The node was left intact; remove the resource from state to acknowledge it.",
	)
}

// ImportState implements resource.ResourceWithImportState. The import ID is
// the node name.
func (r *pveNodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("node"), req, resp)
}

// applyConfig pushes the planned configuration to PVE in three PUT calls.
// /config is the only one required; /dns and /time are best-effort and only
// invoked when at least one of their attrs is non-null.
func (r *pveNodeResource) applyConfig(ctx context.Context, model *pveNodeResourceModel) error {
	node := model.Node.ValueString()
	cfg := pveclient.NodeConfig{
		Description:         model.Description.ValueString(),
		Wakeonlan:           model.WakeOnLAN.ValueString(),
		StartAllOnBootDelay: int(model.StartAllOnBootDelay.ValueInt64()),
		Digest:              model.Digest.ValueString(),
	}
	for _, d := range model.ACMEDomains {
		entry := pveclient.NodeACMEDomain{Domain: d.Domain.ValueString()}
		for _, a := range d.Alias {
			entry.Alias = append(entry.Alias, a.ValueString())
		}
		cfg.ACMEDomains = append(cfg.ACMEDomains, entry)
	}
	if err := r.client.UpdateNodeConfig(ctx, node, cfg); err != nil {
		return fmt.Errorf("update /nodes/%s/config: %w", node, err)
	}

	dns := pveclient.NodeDNS{
		Search: model.DNSSearch.ValueString(),
		DNS1:   model.DNS1.ValueString(),
		DNS2:   model.DNS2.ValueString(),
		DNS3:   model.DNS3.ValueString(),
	}
	if dns.Search != "" || dns.DNS1 != "" || dns.DNS2 != "" || dns.DNS3 != "" {
		if err := r.client.SetNodeDNS(ctx, node, dns); err != nil {
			return fmt.Errorf("update /nodes/%s/dns: %w", node, err)
		}
	}

	if tz := model.Timezone.ValueString(); tz != "" {
		if err := r.client.SetNodeTime(ctx, node, pveclient.NodeTime{Timezone: tz}); err != nil {
			return fmt.Errorf("update /nodes/%s/time: %w", node, err)
		}
	}
	return nil
}

// readInto populates the model from PVE; a 404 on /config removes the
// resource from state via the caller.
func (r *pveNodeResource) readInto(ctx context.Context, model *pveNodeResourceModel) error {
	node := model.Node.ValueString()
	cfg, err := r.client.GetNodeConfig(ctx, node)
	if err != nil {
		return err
	}
	model.Description = types.StringValue(cfg.Description)
	model.WakeOnLAN = types.StringValue(cfg.Wakeonlan)
	model.StartAllOnBootDelay = types.Int64Value(int64(cfg.StartAllOnBootDelay))
	model.Digest = types.StringValue(cfg.Digest)
	if len(cfg.ACMEDomains) == 0 {
		model.ACMEDomains = nil
	} else {
		entries := make([]pveNodeACMEDomainModel, 0, len(cfg.ACMEDomains))
		for _, d := range cfg.ACMEDomains {
			aliases := make([]types.String, 0, len(d.Alias))
			for _, a := range d.Alias {
				aliases = append(aliases, types.StringValue(a))
			}
			entries = append(entries, pveNodeACMEDomainModel{
				Domain: types.StringValue(d.Domain),
				Alias:  aliases,
			})
		}
		model.ACMEDomains = entries
	}

	dns, err := r.client.GetNodeDNS(ctx, node)
	if err != nil {
		return fmt.Errorf("read /nodes/%s/dns: %w", node, err)
	}
	model.DNSSearch = types.StringValue(dns.Search)
	model.DNS1 = types.StringValue(dns.DNS1)
	model.DNS2 = types.StringValue(dns.DNS2)
	model.DNS3 = types.StringValue(dns.DNS3)

	t, err := r.client.GetNodeTime(ctx, node)
	if err != nil {
		return fmt.Errorf("read /nodes/%s/time: %w", node, err)
	}
	model.Timezone = types.StringValue(t.Timezone)
	return nil
}

// isPVEClientNotFound returns true when err is a pveclient *APIError with
// status 404. Used by Read to drop the resource from state.
func isPVEClientNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *pveclient.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
