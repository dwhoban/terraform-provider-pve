// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveNodeServiceAction{}
)

// NewPveNodeServiceAction returns the action implementation.
func NewPveNodeServiceAction() action.Action {
	return &pveNodeServiceAction{}
}

// pveNodeServiceAction performs a lifecycle operation (start, stop,
// restart, reload) on one PVE-managed systemd service
// (POST /nodes/{node}/services/{service}/{operation}) and waits for the
// worker task to finish.
type pveNodeServiceAction struct {
	client *pveclient.Client
}

// pveNodeServiceActionModel is the Terraform-facing shape.
type pveNodeServiceActionModel struct {
	Node      types.String `tfsdk:"node"`
	Service   types.String `tfsdk:"service"`
	Operation types.String `tfsdk:"operation"`
}

// Metadata implements action.Action.
func (a *pveNodeServiceAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeService
}

// Schema implements action.Action.
func (a *pveNodeServiceAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Performs a lifecycle operation on one PVE-managed systemd service (`POST /nodes/{node}/services/{service}/{operation}`) and waits for the worker task to finish. `reload` falls back to a restart server-side when the unit cannot be reloaded. The pinned API restricts `service` to the PVE-managed units (e.g. `pveproxy`, `pvedaemon`, `pve-cluster`, `pve-firewall`, `pvescheduler`, `pvestatd`, `corosync`, `cron`, `postfix`, `sshd`, `chrony`, `ksmtuned`, `lxcfs`, `spiceproxy`, `syslog`, `systemd-journald`, `systemd-timesyncd`, `qmeventd`, `pvefw-logger`, `pve-ha-crm`, `pve-ha-lrm`, `pve-lxc-syscalld`, `proxmox-firewall`); other unit names are rejected by the server.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the service.",
			},
			"service": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Service ID (systemd unit name, e.g. `pveproxy`). See the action description for the units the pinned API accepts.",
			},
			"operation": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Lifecycle operation. Must be one of: `start`, `stop`, `restart`, `reload`.",
				Validators: []validator.String{
					stringvalidator.OneOf(pveclient.NodeServiceOperations...),
				},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeServiceAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeServiceAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeServiceActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_service",
			fmt.Sprintf("The provider client was not configured; cannot run %s on service %s.", config.Operation.ValueString(), config.Service.ValueString()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	if err := a.run(ctx, config, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_service",
			fmt.Sprintf("%s service %s on node %s: %s", config.Operation.ValueString(), config.Service.ValueString(), config.Node.ValueString(), err),
		)
	}
}

// run performs the service operation and waits for the worker task.
func (a *pveNodeServiceAction) run(ctx context.Context, config pveNodeServiceActionModel, progress func(string)) error {
	node, service, operation := config.Node.ValueString(), config.Service.ValueString(), config.Operation.ValueString()
	tflog.Info(ctx, "running service operation", map[string]any{"node": node, "service": service, "operation": operation})
	upid, err := a.client.NodeServiceOp(ctx, node, service, operation)
	if err != nil {
		return fmt.Errorf("starting operation: %w", err)
	}
	waitNode, err := nodeSvcAptNodeFromUpid(upid)
	if err != nil {
		return err
	}
	progress(fmt.Sprintf("Service %s %s on node %s started; waiting for task to finish", service, operation, waitNode))
	tflog.Debug(ctx, "waiting for service operation task", map[string]any{"node": waitNode, "upid": upid})
	if _, err := a.client.WaitForTask(ctx, waitNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return err
	}
	progress(fmt.Sprintf("Service %s %s finished", service, operation))
	return nil
}

// nodeSvcAptNodeFromUpid extracts the node name from a PVE worker task
// UPID (`UPID:<node>:<pid>:<pstart>:<dtype>:<id>:<user>: `).
func nodeSvcAptNodeFromUpid(upid string) (string, error) {
	parts := strings.Split(upid, ":")
	if len(parts) < 2 || parts[0] != "UPID" || parts[1] == "" {
		return "", fmt.Errorf("cannot determine node from task UPID %q", upid)
	}
	return parts[1], nil
}
