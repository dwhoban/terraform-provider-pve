// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strconv"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveContainerSnapshotResource{}
	_ resource.ResourceWithConfigure   = &pveContainerSnapshotResource{}
	_ resource.ResourceWithImportState = &pveContainerSnapshotResource{}
)

// NewPveContainerSnapshotResource returns the resource implementation.
func NewPveContainerSnapshotResource() resource.Resource {
	return &pveContainerSnapshotResource{}
}

// pveContainerSnapshotResource manages one LXC snapshot via
// POST/DELETE /nodes/{node}/lxc/{vmid}/snapshot and PUT
// .../snapshot/{snapname}/config. The snapshot name is immutable; changing
// it forces replacement.
type pveContainerSnapshotResource struct {
	client *pveclient.Client
}

// pveContainerSnapshotResourceModel is the Terraform-facing shape.
type pveContainerSnapshotResourceModel struct {
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vmid"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
	Parent      types.String `tfsdk:"parent"`
}

// Metadata implements resource.Resource.
func (r *pveContainerSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerSnapshot
}

// Schema implements resource.Resource.
func (r *pveContainerSnapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates and destroys one snapshot of an LXC container (`POST/DELETE /nodes/{node}/lxc/{vmid}/snapshot`). " +
			"The snapshot name is immutable: changing `name`, `node`, or `vmid` forces replacement. Only the " +
			"`description` updates in place (`PUT .../snapshot/{snapname}/config`). Requires the `VM.Snapshot` privilege " +
			"on `/vms/{vmid}`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container runs on. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The (unique) ID of the container. Must be between 100 and 999999999. Changing this value forces recreation.",
				Validators:          []validator.Int64{containerActionVMIDValidator()},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the snapshot (upstream `pve-configid`: at most 40 characters). The reserved name `current` refers to the live state and cannot be used. Changing this value forces recreation.",
				Validators:          []validator.String{containerSnapshotNameValidator()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A textual description or comment stored with the snapshot. Updating it rewrites the snapshot metadata only.",
			},
			"snaptime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Unix epoch timestamp of when the snapshot was taken.",
			},
			"parent": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the parent snapshot in the snapshot tree, when one exists.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveContainerSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveContainerSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveContainerSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_container_snapshot", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()
	vmid := plan.VMID.ValueInt64()
	name := plan.Name.ValueString()

	upid, err := r.client.CreateLxcSnapshot(ctx, node, vmid, name, plan.Description.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_container_snapshot", fmt.Sprintf("creating snapshot %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_container_snapshot create", fmt.Sprintf("waiting for snapshot create %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_container_snapshot after create", fmt.Sprintf("reading snapshot %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveContainerSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveContainerSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_container_snapshot", fmt.Sprintf("reading snapshot %s of container %d on %s: %s", state.Name.ValueString(), state.VMID.ValueInt64(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource: only the description is mutable, via
// PUT .../snapshot/{snapname}/config. name, node, and vmid force
// replacement, so any other change never reaches this method.
func (r *pveContainerSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveContainerSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_container_snapshot", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()
	vmid := plan.VMID.ValueInt64()
	name := plan.Name.ValueString()

	if err := r.client.UpdateLxcSnapshotConfig(ctx, node, vmid, name, plan.Description.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating pve_container_snapshot", fmt.Sprintf("updating snapshot %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_container_snapshot after update", fmt.Sprintf("reading snapshot %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveContainerSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveContainerSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_container_snapshot", "provider client is not configured")
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()
	name := state.Name.ValueString()

	upid, err := r.client.DeleteLxcSnapshot(ctx, node, vmid, name, false)
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_container_snapshot", fmt.Sprintf("deleting snapshot %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_container_snapshot delete", fmt.Sprintf("waiting for snapshot delete %s of container %d on %s: %s", name, vmid, node, err))
		return
	}
}

// ImportState parses an import ID of the form `<node>/<vmid>/<name>`.
func (r *pveContainerSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmid, name, err := containerSnapshotParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid pve_container_snapshot import ID",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// readInto refreshes the description and computed fields from
// GET .../snapshot/{snapname}/config; the synthesized 404 for a missing
// snapshot propagates so Read can remove the resource.
func (r *pveContainerSnapshotResource) readInto(ctx context.Context, m *pveContainerSnapshotResourceModel) error {
	cfg, err := r.client.GetLxcSnapshotConfig(ctx, m.Node.ValueString(), m.VMID.ValueInt64(), m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Description = nodeNetworkStringToTF(cfg.Description)
	m.Snaptime = haInt64PtrToTF(cfg.Snaptime)
	m.Parent = nodeNetworkStringToTF(cfg.Parent)
	return nil
}

// containerSnapshotParseImportID splits `<node>/<vmid>/<name>`.
func containerSnapshotParseImportID(id string) (string, int64, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", 0, "", fmt.Errorf("expected import ID of the form <node>/<vmid>/<name> (e.g. pve1/100/pre-upgrade), got %q", id)
	}
	parsed, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("vmid %q is not a number in import ID %q", parts[1], id)
	}
	return parts[0], parsed, parts[2], nil
}

// containerSnapshotNameValidator bounds the snapshot name to the pin's
// `pve-configid` length and rejects the reserved `current` pseudo-snapshot.
func containerSnapshotNameValidator() validator.String {
	return stringvalidator.All(
		stringvalidator.LengthBetween(1, 40),
		stringvalidator.NoneOf("current"),
	)
}
