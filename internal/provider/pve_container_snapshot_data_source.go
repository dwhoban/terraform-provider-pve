// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveContainerSnapshotDataSource{}
	_ datasource.DataSourceWithConfigure = &pveContainerSnapshotDataSource{}
)

// NewPveContainerSnapshotDataSource returns the data source implementation.
func NewPveContainerSnapshotDataSource() datasource.DataSource {
	return &pveContainerSnapshotDataSource{}
}

// pveContainerSnapshotDataSource reads one LXC snapshot from
// GET /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config.
type pveContainerSnapshotDataSource struct {
	client *pveclient.Client
}

// pveContainerSnapshotDataSourceModel is the Terraform-facing shape.
type pveContainerSnapshotDataSourceModel struct {
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vmid"`
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
	Parent      types.String `tfsdk:"parent"`
}

// Metadata implements datasource.DataSource.
func (d *pveContainerSnapshotDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerSnapshot
}

// Schema implements datasource.DataSource.
func (d *pveContainerSnapshotDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one snapshot of an LXC container (`GET /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config`). " +
			"Requires the `VM.Snapshot`, `VM.Snapshot.Rollback`, or `VM.Audit` privilege on `/vms/{vmid}`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container runs on.",
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The (unique) ID of the container. Must be between 100 and 999999999.",
				Validators:          []validator.Int64{containerActionVMIDValidator()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the snapshot (upstream `pve-configid`: at most 40 characters). The reserved name `current` refers to the live state and cannot be read here.",
				Validators:          []validator.String{containerSnapshotNameValidator()},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Synthetic identifier of the form `<node>/<vmid>/<name>`.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The textual description stored with the snapshot.",
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

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveContainerSnapshotDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = containerSnapshotDataSourceConfigure(req, resp)
}

// containerSnapshotDataSourceConfigure extracts the provider client for the
// pve_container_snapshot data source, tolerating the unconfigured phase.
func containerSnapshotDataSourceConfigure(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// Read implements datasource.DataSource.
func (d *pveContainerSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveContainerSnapshotDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_container_snapshot data source", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	vmid := data.VMID.ValueInt64()
	name := data.Name.ValueString()

	cfg, err := d.client.GetLxcSnapshotConfig(ctx, node, vmid, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_container_snapshot data source", fmt.Sprintf("reading snapshot %s of container %d via %s: %s", name, vmid, node, err))
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%d/%s", node, vmid, name))
	data.Description = nodeNetworkStringToTF(cfg.Description)
	data.Snaptime = haInt64PtrToTF(cfg.Snaptime)
	data.Parent = nodeNetworkStringToTF(cfg.Parent)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
