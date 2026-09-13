// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveClusterOptionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveClusterOptionsDataSource{}
)

// NewPveClusterOptionsDataSource returns the data source implementation.
func NewPveClusterOptionsDataSource() datasource.DataSource {
	return &pveClusterOptionsDataSource{}
}

// pveClusterOptionsDataSource reads the datacenter-wide cluster options
// singleton via GET /cluster/options.
type pveClusterOptionsDataSource struct {
	client *pveclient.Client
}

// pveClusterOptionsDataSourceModel is the Terraform-facing shape of the
// data source.
type pveClusterOptionsDataSourceModel struct {
	pveClusterOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// Metadata implements datasource.DataSource.
func (d *pveClusterOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterOptions
}

// Schema implements datasource.DataSource.
func (d *pveClusterOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the datacenter-wide cluster options singleton (`GET /cluster/options`). All attributes are computed; unset options are null. Requires `Sys.Audit` on `/` — without it not all options are returned.",
		Attributes:          clusterOptionsDataSourceAttributes(),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveClusterOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveClusterOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveClusterOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := clusterOptionsReadInto(ctx, d.client, &data.pveClusterOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_options data source",
			fmt.Sprintf("reading cluster options: %s", err),
		)
		return
	}
	data.ID = types.StringValue(pveClusterOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// clusterOptionsDataSourceAttributes renders the full computed attribute
// set.
func clusterOptionsDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := make(map[string]datasourceschema.Attribute, len(clusterOptionsFieldSpecs)+1)
	for _, f := range clusterOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsDataSourceLeaf(f)
	}
	attrs["id"] = datasourceschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Singleton identifier for the datacenter options; always `cluster`.",
	}
	return attrs
}

// clusterOptionsDataSourceLeaf renders one spec entry as a computed data
// source attribute; validators do not apply to computed-only values.
func clusterOptionsDataSourceLeaf(f clusterOptionsField) datasourceschema.Attribute {
	switch f.Kind {
	case clusterOptionsKindBool:
		return datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: clusterOptionsLeafDescription(f)}
	case clusterOptionsKindInt64:
		return datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: clusterOptionsLeafDescription(f)}
	case clusterOptionsKindFloat64:
		return datasourceschema.Float64Attribute{Computed: true, MarkdownDescription: clusterOptionsLeafDescription(f)}
	case clusterOptionsKindObject:
		attrs := make(map[string]datasourceschema.Attribute, len(f.Fields))
		for _, inner := range f.Fields {
			attrs[inner.Name] = clusterOptionsDataSourceLeaf(inner)
		}
		return datasourceschema.SingleNestedAttribute{Computed: true, MarkdownDescription: f.Description, Attributes: attrs}
	default:
		return datasourceschema.StringAttribute{Computed: true, MarkdownDescription: clusterOptionsLeafDescription(f)}
	}
}
