// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
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

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveCephMonResource{}
	_ resource.ResourceWithConfigure   = &pveCephMonResource{}
	_ resource.ResourceWithImportState = &pveCephMonResource{}
)

// NewPveCephMonResource returns the resource implementation.
func NewPveCephMonResource() resource.Resource {
	return &pveCephMonResource{}
}

// pveCephMonResource manages one Ceph monitor via
// POST/DELETE /nodes/{node}/ceph/mon/{monid}. The monitor is created on
// `node`; every non-key attribute forces replacement because the pin
// defines no monitor update verb.
type pveCephMonResource struct {
	client *pveclient.Client
}

// pveCephMonResourceModel is the Terraform-facing shape.
type pveCephMonResourceModel struct {
	Node        types.String `tfsdk:"node"`
	MonID       types.String `tfsdk:"monid"`
	MonAddress  types.String `tfsdk:"mon_address"`
	Addr        types.String `tfsdk:"addr"`
	Host        types.String `tfsdk:"host"`
	State       types.String `tfsdk:"state"`
	Rank        types.Int64  `tfsdk:"rank"`
	InQuorum    types.Bool   `tfsdk:"in_quorum"`
	Service     types.Bool   `tfsdk:"service"`
	CephVersion types.String `tfsdk:"ceph_version"`
}

// Metadata implements resource.Resource.
func (r *pveCephMonResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephMon
}

// Schema implements resource.Resource.
func (r *pveCephMonResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates and destroys one Ceph monitor on a node (`POST/DELETE /nodes/{node}/ceph/mon/{monid}`). PVE auto-creates a Manager for the first monitor. All attributes force replacement because the pin defines no monitor update verb.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node the monitor is created on; PVE defaults the monitor id to this node name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"monid": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The ID for the monitor (pattern: alphanumeric with inner alphanumerics or dashes, max 200). When omitted, PVE defaults it to the node name, which the provider resolves into state after create.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mon_address": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Overwrites the autodetected monitor IP address(es); must be in the public network(s) of Ceph (pin `mon-address`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Ceph-formatted monitor address as advertised by the monitor (typically `IP:PORT/NONCE`, possibly a messenger-v2 vector).",
			},
			"host": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host the monitor runs on.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Run state of the monitor: `running` (in quorum), `stopped` (systemd unit configured but daemon not visible to the cluster), or `unknown` (no rados access).",
			},
			"rank": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Rank of the monitor within the mon map.",
			},
			"in_quorum": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the monitor is part of the current quorum.",
			},
			"service": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether a `ceph-mon@<id>` systemd unit is enabled on the hosting node.",
			},
			"ceph_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full Ceph version string of the monitor daemon.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveCephMonResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = cephConfigureResource(req, resp)
}

// cephMonEffectiveID resolves the monitor id the create request targets:
// the configured monid, or the node name (the pin's monid default).
func cephMonEffectiveID(m pveCephMonResourceModel) string {
	if m.MonID.IsNull() || m.MonID.IsUnknown() {
		return m.Node.ValueString()
	}
	return m.MonID.ValueString()
}

// Create implements resource.Resource.
func (r *pveCephMonResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveCephMonResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_mon", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()
	effectiveID := cephMonEffectiveID(plan)

	upid, err := r.client.CreateCephMon(ctx, node, plan.MonID.ValueString(), plan.MonAddress.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_mon", fmt.Sprintf("creating monitor %s on %s: %s", effectiveID, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_mon create", fmt.Sprintf("waiting for monitor create %s on %s: %s", effectiveID, node, err))
		return
	}
	// Resolve the computed monid to the id the request actually targeted.
	plan.MonID = types.StringValue(effectiveID)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_mon after create", fmt.Sprintf("reading monitor %s on %s: %s", effectiveID, node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveCephMonResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveCephMonResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_ceph_mon", fmt.Sprintf("reading monitor %s on %s: %s", state.MonID.ValueString(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op at the API level: the pin defines no monitor update
// verb. Plan modifiers force replacement for every attribute, so this
// method should never run in practice.
func (r *pveCephMonResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveCephMonResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"pve_ceph_mon update ignored",
		"PVE offers no in-place monitor reconfiguration; the plan should have forced replacement.",
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveCephMonResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveCephMonResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_ceph_mon", "provider client is not configured")
		return
	}
	upid, err := r.client.DestroyCephMon(ctx, state.Node.ValueString(), state.MonID.ValueString())
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_ceph_mon", fmt.Sprintf("destroying monitor %s on %s: %s", state.MonID.ValueString(), state.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, state.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_mon delete", fmt.Sprintf("waiting for monitor destroy %s on %s: %s", state.MonID.ValueString(), state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:<monid>`.
func (r *pveCephMonResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_ceph_mon import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<monid>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monid"), parts[1])...)
}

// readInto refreshes the computed fields from the monitor listing; the
// synthesized 404 from GetCephMon propagates so Read can remove the
// resource.
func (r *pveCephMonResource) readInto(ctx context.Context, m *pveCephMonResourceModel) error {
	mon, err := r.client.GetCephMon(ctx, m.Node.ValueString(), m.MonID.ValueString())
	if err != nil {
		return err
	}
	m.Addr = nodeNetworkStringToTF(mon.Addr)
	m.Host = nodeNetworkStringToTF(mon.Host)
	m.State = nodeNetworkStringToTF(mon.State)
	m.Rank = cephInt64PtrToTF(mon.Rank)
	m.InQuorum = nodeNetworkBoolPtrToTF(mon.Quorum)
	m.Service = nodeNetworkBoolPtrToTF(mon.Service)
	m.CephVersion = nodeNetworkStringToTF(mon.CephVersion)
	return nil
}
