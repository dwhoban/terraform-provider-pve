// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveCephPoolDataSource{}
	_ datasource.DataSourceWithConfigure = &pveCephPoolDataSource{}
)

// NewPveCephPoolDataSource returns the data source implementation.
func NewPveCephPoolDataSource() datasource.DataSource {
	return &pveCephPoolDataSource{}
}

// pveCephPoolDataSource reads a single Ceph pool from the pool listing
// (GET /nodes/{node}/ceph/pool).
type pveCephPoolDataSource struct {
	client *pveclient.Client
}

// pveCephPoolDataSourceModel is the Terraform-facing shape.
type pveCephPoolDataSourceModel struct {
	ID                  types.String  `tfsdk:"id"`
	Node                types.String  `tfsdk:"node"`
	Name                types.String  `tfsdk:"name"`
	PoolID              types.Int64   `tfsdk:"pool_id"`
	PoolType            types.String  `tfsdk:"pool_type"`
	Size                types.Int64   `tfsdk:"size"`
	MinSize             types.Int64   `tfsdk:"min_size"`
	PGNum               types.Int64   `tfsdk:"pg_num"`
	PGNumMin            types.Int64   `tfsdk:"pg_num_min"`
	PGNumFinal          types.Int64   `tfsdk:"pg_num_final"`
	PGAutoscaleMode     types.String  `tfsdk:"pg_autoscale_mode"`
	CrushRuleID         types.Int64   `tfsdk:"crush_rule_id"`
	CrushRuleName       types.String  `tfsdk:"crush_rule_name"`
	TargetSize          types.Int64   `tfsdk:"target_size"`
	TargetSizeRatio     types.Float64 `tfsdk:"target_size_ratio"`
	BytesUsed           types.Int64   `tfsdk:"bytes_used"`
	PercentUsed         types.Float64 `tfsdk:"percent_used"`
	ApplicationMetadata types.String  `tfsdk:"application_metadata"`
	AutoscaleStatus     types.String  `tfsdk:"autoscale_status"`
}

// Metadata implements datasource.DataSource.
func (d *pveCephPoolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephPool
}

// Schema implements datasource.DataSource.
func (d *pveCephPoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single Ceph pool and its settings from `GET /nodes/{node}/ceph/pool`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Management node the listing request is issued against.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the pool (`pool_name` in the listing).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in the form `<node>:<name>`.",
			},
			"pool_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Numeric pool id assigned by Ceph.",
			},
			"pool_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pool type: `replicated` for n-way replication, `erasure` for an erasure-coded pool, or `unknown`.",
			},
			"size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Replication factor (target number of object replicas).",
			},
			"min_size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Minimum number of replicas required to accept writes.",
			},
			"pg_num": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Current placement-group count.",
			},
			"pg_num_min": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Minimum placement-group count the pg_autoscaler may choose.",
			},
			"pg_num_final": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Optimal placement-group count computed by the pg_autoscaler.",
			},
			"pg_autoscale_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Placement-group autoscaler mode: `on`, `warn`, or `off`.",
			},
			"crush_rule_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Numeric id of the CRUSH rule used by this pool.",
			},
			"crush_rule_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable name of the CRUSH rule used by this pool.",
			},
			"target_size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Operator-supplied target size in bytes hinting the pg_autoscaler.",
			},
			"target_size_ratio": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Operator-supplied target ratio of total pool capacity hinting the pg_autoscaler.",
			},
			"bytes_used": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Bytes currently used in the pool; absent when no usage statistics are reported.",
			},
			"percent_used": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Percentage of pool capacity currently used; absent when no usage statistics are reported.",
			},
			"application_metadata": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw JSON object of application tags attached to the pool (mapping of application name to its metadata); empty when unset.",
			},
			"autoscale_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw JSON pg_autoscaler status object for this pool; the shape varies between Ceph releases.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveCephPoolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = cephConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveCephPoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveCephPoolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_pool data source", "provider client is not configured")
		return
	}

	pool, err := d.client.GetCephPool(ctx, data.Node.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_pool data source", fmt.Sprintf("reading pool %s via %s: %s", data.Name.ValueString(), data.Node.ValueString(), err))
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Node.ValueString(), data.Name.ValueString()))
	data.PoolID = types.Int64Value(int64(pool.Pool))
	data.PoolType = nodeNetworkStringToTF(pool.Type)
	data.Size = types.Int64Value(int64(pool.Size))
	data.MinSize = types.Int64Value(int64(pool.MinSize))
	data.PGNum = types.Int64Value(int64(pool.PGNum))
	data.PGNumMin = cephInt64OptToTF(pool.PGNumMin)
	data.PGNumFinal = cephInt64OptToTF(pool.PGNumFinal)
	data.PGAutoscaleMode = nodeNetworkStringToTF(pool.PGAutoscaleMode)
	data.CrushRuleID = types.Int64Value(int64(pool.CrushRule))
	data.CrushRuleName = nodeNetworkStringToTF(pool.CrushRuleName)
	if pool.TargetSize != nil {
		data.TargetSize = types.Int64Value(*pool.TargetSize)
	} else {
		data.TargetSize = types.Int64Null()
	}
	if pool.TargetSizeRatio != nil {
		data.TargetSizeRatio = types.Float64Value(*pool.TargetSizeRatio)
	} else {
		data.TargetSizeRatio = types.Float64Null()
	}
	if pool.BytesUsed != nil {
		data.BytesUsed = types.Int64Value(*pool.BytesUsed)
	} else {
		data.BytesUsed = types.Int64Null()
	}
	if pool.PercentUsed != nil {
		data.PercentUsed = types.Float64Value(*pool.PercentUsed)
	} else {
		data.PercentUsed = types.Float64Null()
	}
	data.ApplicationMetadata = cephPoolRawJSONStringTF(pool.ApplicationMetadata)
	data.AutoscaleStatus = cephPoolRawJSONStringTF(pool.AutoscaleStatus)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// cephPoolRawJSONStringTF renders a raw JSON object field for state; absent
// payloads become null.
func cephPoolRawJSONStringTF(raw json.RawMessage) types.String {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return types.StringNull()
	}
	return types.StringValue(string(trimmed))
}
