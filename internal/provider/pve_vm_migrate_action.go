// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveVmMigrateAction{}
	_ action.ActionWithConfigure = &pveVmMigrateAction{}
)

// NewPveVmMigrateAction returns the action implementation.
func NewPveVmMigrateAction() action.Action {
	return &pveVmMigrateAction{}
}

// pveVmMigrateAction migrates a QEMU VM to another node
// (POST /nodes/{node}/qemu/{vmid}/migrate) and waits for the task.
type pveVmMigrateAction struct {
	client *pveclient.Client
}

// pveVmMigrateActionModel is the Terraform-facing shape.
type pveVmMigrateActionModel struct {
	Node             types.String `tfsdk:"node"`
	VMID             types.Int64  `tfsdk:"vmid"`
	Target           types.String `tfsdk:"target"`
	TargetStorage    types.String `tfsdk:"target_storage"`
	Bandwidth        types.Int64  `tfsdk:"bandwidth"`
	MigrationType    types.String `tfsdk:"migration_type"`
	MigrationNetwork types.String `tfsdk:"migration_network"`
	Online           types.Bool   `tfsdk:"online"`
	WithLocalDisks   types.Bool   `tfsdk:"with_local_disks"`
	Force            types.Bool   `tfsdk:"force"`
}

// Metadata implements action.Action.
func (a *pveVmMigrateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmMigrate
}

// Schema implements action.Action.
func (a *pveVmMigrateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Migrates a QEMU VM to another cluster node (`POST /nodes/{node}/qemu/{vmid}/migrate`). The action waits for the migration task to finish. For running guests, `online` performs a live migration.",
		Attributes: map[string]schema.Attribute{
			"node": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the VM currently runs on.",
			},
			"vmid": &schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The VM identifier. Must be between 100 and 999999999.",
				Validators:          vmActionVMIDValidators,
			},
			"target": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The target cluster node.",
			},
			"target_storage": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Overrides the target storage for local disks.",
			},
			"bandwidth": &schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Limits the migration bandwidth in KiB/s (`bwlimit`). A value of `0` resets the limit to the global setting.",
			},
			"migration_type": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The migration traffic type. Must be one of `secure`, `insecure`.",
				Validators:          []validator.String{stringvalidator.OneOf("secure", "insecure")},
			},
			"migration_network": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The CIDR of the (sub)network used for migration traffic.",
			},
			"online": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If set, performs a live migration of a running VM.",
			},
			"with_local_disks": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If set, local disks are migrated to the target node.",
			},
			"force": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force the migration even against anti-affinity rules. Only root may use this option.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveVmMigrateAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	a.client = client
}

// Invoke implements action.Action.
func (a *pveVmMigrateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmMigrateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_migrate",
			fmt.Sprintf("The provider client was not configured; cannot migrate VM %d.", config.VMID.ValueInt64()),
		)
		return
	}

	opts := pveclient.QemuVMMigrateOptions{Target: config.Target.ValueString()}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	if !config.TargetStorage.IsNull() && !config.TargetStorage.IsUnknown() {
		opts.TargetStorage = config.TargetStorage.ValueString()
	}
	if !config.Bandwidth.IsNull() && !config.Bandwidth.IsUnknown() {
		v := config.Bandwidth.ValueInt64()
		opts.Bandwidth = &v
	}
	if !config.MigrationType.IsNull() && !config.MigrationType.IsUnknown() {
		opts.MigrationType = config.MigrationType.ValueString()
	}
	if !config.MigrationNetwork.IsNull() && !config.MigrationNetwork.IsUnknown() {
		opts.MigrationNetwork = config.MigrationNetwork.ValueString()
	}
	if !config.Online.IsNull() && !config.Online.IsUnknown() {
		v := config.Online.ValueBool()
		opts.Online = &v
	}
	if !config.WithLocalDisks.IsNull() && !config.WithLocalDisks.IsUnknown() {
		v := config.WithLocalDisks.ValueBool()
		opts.WithLocalDisks = &v
	}
	if !config.Force.IsNull() && !config.Force.IsUnknown() {
		v := config.Force.ValueBool()
		opts.Force = &v
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	tflog.Info(ctx, "migrating VM", map[string]any{"node": node, "vmid": vmid, "target": config.Target.ValueString()})
	upid, err := a.client.MigrateQemuVM(ctx, node, vmid, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_migrate",
			fmt.Sprintf("migrating VM %d from node %s to %s: %s", vmid, node, config.Target.ValueString(), err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_migrate",
			fmt.Sprintf("waiting for migration of VM %d from node %s to %s: %s", vmid, node, config.Target.ValueString(), err),
		)
	}
}
