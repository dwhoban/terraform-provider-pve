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
	_ datasource.DataSource              = &pveCustomCPUModelDataSource{}
	_ datasource.DataSourceWithConfigure = &pveCustomCPUModelDataSource{}
)

// NewPveCustomCPUModelDataSource returns the data source implementation.
func NewPveCustomCPUModelDataSource() datasource.DataSource {
	return &pveCustomCPUModelDataSource{}
}

// pveCustomCPUModelDataSource reads a single custom CPU model definition
// (GET /cluster/qemu/custom-cpu-models/{cputype}).
type pveCustomCPUModelDataSource struct {
	client *pveclient.Client
}

// pveCustomCPUModelDataSourceModel is the Terraform-facing shape.
type pveCustomCPUModelDataSourceModel struct {
	Name          types.String `tfsdk:"name"`
	ReportedModel types.String `tfsdk:"reported_model"`
	Flags         types.String `tfsdk:"flags"`
	GuestPhysBits types.Int64  `tfsdk:"guest_phys_bits"`
	Hidden        types.Bool   `tfsdk:"hidden"`
	HvVendorID    types.String `tfsdk:"hv_vendor_id"`
	Level         types.Int64  `tfsdk:"level"`
	PhysBits      types.String `tfsdk:"phys_bits"`
	Digest        types.String `tfsdk:"digest"`
}

// Metadata implements datasource.DataSource.
func (d *pveCustomCPUModelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCustomCpuModel
}

// Schema implements datasource.DataSource.
func (d *pveCustomCPUModelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single custom CPU model definition (`GET /cluster/qemu/custom-cpu-models/{cputype}`); VMs reference it as `custom-<name>`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the custom CPU model to read (the pin's `cputype` parameter, where the `custom-` prefix is optional).",
			},
			"reported_model": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "CPU model and vendor reported to the guest.",
			},
			"flags": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Additional CPU flags as a `;`-separated list of `+FLAG`/`-FLAG` entries.",
			},
			"guest_phys_bits": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of physical address bits available to the guest.",
			},
			"hidden": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the vCPU hides its KVM virtualization identity.",
			},
			"hv_vendor_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Hyper-V vendor ID.",
			},
			"level": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum input value for the basic CPUID leaves the guest can query.",
			},
			"phys_bits": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Physical memory address bits reported to the guest OS (`8`-`64` or `host`).",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the custom CPU model configuration.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveCustomCPUModelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveCustomCPUModelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveCustomCPUModelDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_custom_cpu_model data source", "provider client is not configured")
		return
	}
	model, err := d.client.GetCustomCPUModel(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_custom_cpu_model data source",
			fmt.Sprintf("reading custom CPU model %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	state.ReportedModel = nodeNetworkStringToTF(model.ReportedModel)
	state.Flags = nodeNetworkStringToTF(model.Flags)
	state.GuestPhysBits = haInt64PtrToTF(model.GuestPhysBits)
	state.Hidden = nodeNetworkBoolPtrToTF(model.Hidden)
	state.HvVendorID = nodeNetworkStringToTF(model.HVVendorID)
	state.Level = haInt64PtrToTF(model.Level)
	state.PhysBits = nodeNetworkStringToTF(model.PhysBits)
	state.Digest = nodeNetworkStringToTF(model.Digest)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
