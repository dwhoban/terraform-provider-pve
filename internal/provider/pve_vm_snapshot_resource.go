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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveVmSnapshotResource{}
	_ resource.ResourceWithConfigure   = &pveVmSnapshotResource{}
	_ resource.ResourceWithImportState = &pveVmSnapshotResource{}
)

// NewPveVmSnapshotResource returns the resource implementation.
func NewPveVmSnapshotResource() resource.Resource {
	return &pveVmSnapshotResource{}
}

// pveVmSnapshotResource manages a single QEMU VM snapshot
// (/nodes/{node}/qemu/{vmid}/snapshot). Snapshots are immutable apart from
// their description, which the resource also treats as replacement.
type pveVmSnapshotResource struct {
	client *pveclient.Client
}

// pveVmSnapshotResourceModel is the Terraform-facing shape.
type pveVmSnapshotResourceModel struct {
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vmid"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
	Parent      types.String `tfsdk:"parent"`
	Vmstate     types.Bool   `tfsdk:"vmstate"`
}

// Metadata implements resource.Resource.
func (r *pveVmSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmSnapshot
}

// Schema implements resource.Resource.
func (r *pveVmSnapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single QEMU VM snapshot (`POST /nodes/{node}/qemu/{vmid}/snapshot`). Snapshot creation and deletion run as cluster tasks; this resource waits for them to finish. Snapshots are immutable: changing any attribute replaces the snapshot. Unmodeled PVE snapshot properties are never sent and never removed.",
		Attributes: map[string]schema.Attribute{
			"node": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the VM.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vmid": &schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The VM identifier. Must be between 100 and 999999999.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"name": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The snapshot identifier (upstream `snapname`, max 40 characters). The reserved name `current` is not allowed.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": &schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "A textual description or comment. Changing it replaces the snapshot.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"snaptime": &schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The snapshot creation time as a Unix epoch in seconds.",
			},
			"parent": &schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The parent snapshot identifier, if the snapshot participates in a snapshot tree.",
			},
			"vmstate": &schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the snapshot includes the VM RAM state.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveVmSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveVmSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveVmSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_vm_snapshot", "provider client is not configured")
		return
	}
	opts := pveclient.QemuVMSnapshotOptions{}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		opts.Description = plan.Description.ValueString()
	}
	node := plan.Node.ValueString()
	vmid := plan.VMID.ValueInt64()
	name := plan.Name.ValueString()
	tflog.Info(ctx, "creating VM snapshot", map[string]any{"node": node, "vmid": vmid, "name": name})
	upid, err := r.client.CreateQemuVMSnapshot(ctx, node, vmid, name, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_vm_snapshot",
			fmt.Sprintf("creating snapshot %s of VM %d on node %s: %s", name, vmid, node, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, r.client, upid, func(string) {}); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_vm_snapshot",
			fmt.Sprintf("waiting for creation of snapshot %s of VM %d on node %s: %s", name, vmid, node, err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_vm_snapshot after create",
			fmt.Sprintf("reading snapshot %s of VM %d on node %s: %s", name, vmid, node, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveVmSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveVmSnapshotResourceModel
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
			"Error reading pve_vm_snapshot",
			fmt.Sprintf("reading snapshot %s of VM %d on node %s: %s", state.Name.ValueString(), state.VMID.ValueInt64(), state.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Every attribute is ForceNew, so the
// framework replaces instead of updating; this is a defensive no-op that
// re-reads the snapshot.
func (r *pveVmSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveVmSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_vm_snapshot after update",
			fmt.Sprintf("reading snapshot %s of VM %d on node %s: %s", plan.Name.ValueString(), plan.VMID.ValueInt64(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveVmSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveVmSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()
	name := state.Name.ValueString()
	tflog.Info(ctx, "deleting VM snapshot", map[string]any{"node": node, "vmid": vmid, "name": name})
	upid, err := r.client.DeleteQemuVMSnapshot(ctx, node, vmid, name, nil)
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_vm_snapshot",
			fmt.Sprintf("deleting snapshot %s of VM %d on node %s: %s", name, vmid, node, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, r.client, upid, func(string) {}); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting pve_vm_snapshot",
			fmt.Sprintf("waiting for deletion of snapshot %s of VM %d on node %s: %s", name, vmid, node, err),
		)
	}
}

// ImportState parses an import ID of the form `<node>/<vmid>/<name>`.
func (r *pveVmSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_vm_snapshot import ID",
			fmt.Sprintf("import ID must be `<node>/<vmid>/<name>`, got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[2])...)
}

// readInto refreshes the model from PVE.
func (r *pveVmSnapshotResource) readInto(ctx context.Context, m *pveVmSnapshotResourceModel) error {
	snap, err := r.client.GetQemuVMSnapshot(ctx, m.Node.ValueString(), m.VMID.ValueInt64(), m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Description = nodeNetworkStringToTF(snap.Description)
	m.Parent = nodeNetworkStringToTF(snap.Parent)
	m.Snaptime = haInt64PtrToTF(snap.Snaptime)
	m.Vmstate = nodeNetworkBoolPtrToTF(snap.Vmstate)
	return nil
}
