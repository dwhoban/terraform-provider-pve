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
	_ datasource.DataSource              = &pveHaStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHaStatusDataSource{}
)

// NewPveHaStatusDataSource returns the data source implementation.
func NewPveHaStatusDataSource() datasource.DataSource {
	return &pveHaStatusDataSource{}
}

// haConfigureDataSource extracts the shared client from provider data for
// HA data sources. Nil provider data leaves the data source unconfigured
// (unit tests).
func haConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
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

// pveHaStatusDataSource reads the HA stack status
// (GET /cluster/ha/status/current and GET /cluster/ha/status/manager_status).
type pveHaStatusDataSource struct {
	client *pveclient.Client
}

// pveHaStatusDataSourceModel is the Terraform-facing shape.
type pveHaStatusDataSourceModel struct {
	ID            types.String            `tfsdk:"id"`
	Quorate       types.Bool              `tfsdk:"quorate"`
	QuorumStatus  types.String            `tfsdk:"quorum_status"`
	MasterNode    types.String            `tfsdk:"master_node"`
	Nodes         types.List              `tfsdk:"nodes"`
	ManagerStatus types.String            `tfsdk:"manager_status"`
	Entries       []pveHaStatusEntryModel `tfsdk:"entries"`
}

// pveHaStatusEntryModel mirrors one status entry. Which fields carry a
// value depends on the entry type (`quorum`, `master`, `lrm`, `service`,
// `fencing`).
type pveHaStatusEntryModel struct {
	ID            types.String `tfsdk:"id"`
	Type          types.String `tfsdk:"type"`
	Node          types.String `tfsdk:"node"`
	Status        types.String `tfsdk:"status"`
	State         types.String `tfsdk:"state"`
	CRMState      types.String `tfsdk:"crm_state"`
	RequestState  types.String `tfsdk:"request_state"`
	SID           types.String `tfsdk:"sid"`
	ArmedState    types.String `tfsdk:"armed_state"`
	ResourceMode  types.String `tfsdk:"resource_mode"`
	Quorate       types.Bool   `tfsdk:"quorate"`
	Timestamp     types.Int64  `tfsdk:"timestamp"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
	Failback      types.Bool   `tfsdk:"failback"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
}

// Metadata implements datasource.DataSource.
func (d *pveHaStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaStatus
}

// Schema implements datasource.DataSource.
func (d *pveHaStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cluster-wide HA stack status from `GET /cluster/ha/status/current` and `GET /cluster/ha/status/manager_status`, including quorum, the active master, per-node LRM entries, and HA-managed services.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the HA status index.",
			},
			"quorate": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the cluster is currently quorate, taken from the `quorum` status entry.",
			},
			"quorum_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw status string of the `quorum` entry (e.g. `OK`).",
			},
			"master_node": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Node currently running the HA master CRM, taken from the `master` status entry.",
			},
			"nodes": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Distinct node names seen across the status entries (master, LRM, and service entries).",
			},
			"manager_status": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Raw HA manager status document from `GET /cluster/ha/status/manager_status`, serialized as JSON text. " +
					"The pin declares this response only as a generic object (the CRM's internal status document), so the field set is version-dependent.",
			},
			"entries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every status entry. Field relevance depends on the entry `type`: `quorum`, `master`, `lrm`, `service`, or `fencing`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Status entry ID (e.g. `quorum`, `master`, `lrm:pve1`, `service:vm:100`)."},
						"type":           schema.StringAttribute{Computed: true, MarkdownDescription: "Entry type: `quorum`, `master`, `lrm`, `service`, or `fencing`."},
						"node":           schema.StringAttribute{Computed: true, MarkdownDescription: "Node associated with the status entry."},
						"status":         schema.StringAttribute{Computed: true, MarkdownDescription: "Status of the entry (value depends on the type)."},
						"state":          schema.StringAttribute{Computed: true, MarkdownDescription: "For type `service`: verbose service state."},
						"crm_state":      schema.StringAttribute{Computed: true, MarkdownDescription: "For type `service`: service state as seen by the CRM."},
						"request_state":  schema.StringAttribute{Computed: true, MarkdownDescription: "For type `service`: requested service state."},
						"sid":            schema.StringAttribute{Computed: true, MarkdownDescription: "For type `service`: HA resource ID."},
						"armed_state":    schema.StringAttribute{Computed: true, MarkdownDescription: "For type `fencing`: whether HA is `armed`, `standby`, `disarming`, or `disarmed`."},
						"resource_mode":  schema.StringAttribute{Computed: true, MarkdownDescription: "For type `fencing`: how resources are handled while disarmed (`freeze` or `ignore`)."},
						"quorate":        schema.BoolAttribute{Computed: true, MarkdownDescription: "For type `quorum`: whether the cluster is quorate."},
						"timestamp":      schema.Int64Attribute{Computed: true, MarkdownDescription: "For types `lrm` and `master`: timestamp of the status information."},
						"max_restart":    schema.Int64Attribute{Computed: true, MarkdownDescription: "For type `service`: maximal restart tries on a node."},
						"max_relocate":   schema.Int64Attribute{Computed: true, MarkdownDescription: "For type `service`: maximal relocate tries."},
						"failback":       schema.BoolAttribute{Computed: true, MarkdownDescription: "For type `service`: whether the resource migrates back to a higher-priority node."},
						"auto_rebalance": schema.BoolAttribute{Computed: true, MarkdownDescription: "For type `service`: whether the resource may be migrated during automatic rebalancing."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveHaStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHaStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveHaStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ha_status", "provider client is not configured")
		return
	}

	entries, err := d.client.GetHAStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_status",
			fmt.Sprintf("reading HA status: %s", err),
		)
		return
	}

	// The manager status document is optional: a cluster whose HA stack
	// never ran may not expose it, which must not fail the whole read.
	managerStatus := types.StringNull()
	raw, err := d.client.GetHAManagerStatus(ctx)
	switch {
	case err == nil:
		managerStatus = types.StringValue(string(raw))
	case isPVEClientNotFound(err):
		// Leave manager_status null.
	default:
		resp.Diagnostics.AddError(
			"Error reading pve_ha_status",
			fmt.Sprintf("reading HA manager status: %s", err),
		)
		return
	}

	data.ID = types.StringValue("ha_status")
	data.ManagerStatus = managerStatus
	data.Entries = haStatusEntriesToTF(entries)

	quorate := types.BoolNull()
	quorumStatus := types.StringNull()
	masterNode := types.StringNull()
	nodeSet := map[string]bool{}
	var nodes []string
	for _, e := range entries {
		if e.Type == "quorum" {
			quorate = nodeNetworkBoolPtrToTF(e.Quorate)
			quorumStatus = nodeNetworkStringToTF(e.Status)
		}
		if e.Type == "master" {
			masterNode = nodeNetworkStringToTF(e.Node)
		}
		if e.Node != "" && !nodeSet[e.Node] {
			nodeSet[e.Node] = true
			nodes = append(nodes, e.Node)
		}
	}
	data.Quorate = quorate
	data.QuorumStatus = quorumStatus
	data.MasterNode = masterNode
	data.Nodes = listStringToTF(nodes)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// haStatusEntriesToTF projects client status entries into the row models.
func haStatusEntriesToTF(entries []pveclient.HAStatusEntry) []pveHaStatusEntryModel {
	rows := make([]pveHaStatusEntryModel, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, pveHaStatusEntryModel{
			ID:            types.StringValue(e.ID),
			Type:          types.StringValue(e.Type),
			Node:          nodeNetworkStringToTF(e.Node),
			Status:        nodeNetworkStringToTF(e.Status),
			State:         nodeNetworkStringToTF(e.State),
			CRMState:      nodeNetworkStringToTF(e.CRMState),
			RequestState:  nodeNetworkStringToTF(e.RequestState),
			SID:           nodeNetworkStringToTF(e.SID),
			ArmedState:    nodeNetworkStringToTF(e.ArmedState),
			ResourceMode:  nodeNetworkStringToTF(e.ResourceMode),
			Quorate:       nodeNetworkBoolPtrToTF(e.Quorate),
			Timestamp:     haInt64ToTF(e.Timestamp),
			MaxRestart:    haInt64PtrToTF(e.MaxRestart),
			MaxRelocate:   haInt64PtrToTF(e.MaxRelocate),
			Failback:      nodeNetworkBoolPtrToTF(e.Failback),
			AutoRebalance: nodeNetworkBoolPtrToTF(e.AutoRebalance),
		})
	}
	return rows
}

// haInt64ToTF maps zero to null (timestamps are absent as zero).
func haInt64ToTF(i int64) types.Int64 {
	if i == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(i)
}

// haInt64PtrToTF maps a nil pointer to null.
func haInt64PtrToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
