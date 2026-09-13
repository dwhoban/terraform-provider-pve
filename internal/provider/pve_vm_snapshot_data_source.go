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
	_ datasource.DataSource              = &pveVmSnapshotDataSource{}
	_ datasource.DataSourceWithConfigure = &pveVmSnapshotDataSource{}
)

// NewPveVmSnapshotDataSource returns the data source implementation.
func NewPveVmSnapshotDataSource() datasource.DataSource {
	return &pveVmSnapshotDataSource{}
}

// pveVmSnapshotDataSource reads a single QEMU VM snapshot
// (GET /nodes/{node}/qemu/{vmid}/snapshot/{snapname}).
type pveVmSnapshotDataSource struct {
	client *pveclient.Client
}

// pveVmSnapshotDataSourceModel is the Terraform-facing shape.
type pveVmSnapshotDataSourceModel struct {
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vmid"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
	Parent      types.String `tfsdk:"parent"`
	Vmstate     types.Bool   `tfsdk:"vmstate"`
}

// Metadata implements datasource.DataSource.
func (d *pveVmSnapshotDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmSnapshot
}

// Schema implements datasource.DataSource.
func (d *pveVmSnapshotDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single QEMU VM snapshot (`GET /nodes/{node}/qemu/{vmid}/snapshot/{snapname}`).",
		Attributes: map[string]schema.Attribute{
			"node": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the VM.",
			},
			"vmid": &schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The VM identifier. Must be between 100 and 999999999.",
			},
			"name": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The snapshot identifier (upstream `snapname`).",
			},
			"description": &schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The snapshot description or comment.",
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

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveVmSnapshotDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveVmSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveVmSnapshotDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Error reading pve_vm_snapshot data source",
			fmt.Sprintf("provider client is not configured; cannot read snapshot %s of VM %d", state.Name.ValueString(), state.VMID.ValueInt64()),
		)
		return
	}
	snap, err := d.client.GetQemuVMSnapshot(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_vm_snapshot data source",
			fmt.Sprintf("reading snapshot %s of VM %d on node %s: %s", state.Name.ValueString(), state.VMID.ValueInt64(), state.Node.ValueString(), err),
		)
		return
	}
	state.Description = nodeNetworkStringToTF(snap.Description)
	state.Parent = nodeNetworkStringToTF(snap.Parent)
	state.Snaptime = haInt64PtrToTF(snap.Snaptime)
	state.Vmstate = nodeNetworkBoolPtrToTF(snap.Vmstate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
