// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// genericDataSource implements every generated data source: each declared
// read populates its ResultName attribute (or merges into top-level
// computed attributes) from one GET endpoint.
type genericDataSource struct {
	desc   *TypeDescriptor
	client *pveclient.Client
}

var (
	_ datasource.DataSource              = (*genericDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*genericDataSource)(nil)
)

// NewGenericDataSource returns the data source constructor for a descriptor.
func NewGenericDataSource(d *TypeDescriptor) func() datasource.DataSource {
	return func() datasource.DataSource { return &genericDataSource{desc: d} }
}

func (d *genericDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = d.desc.Name
}

func (d *genericDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = buildDataSourceSchema(d.desc)
}

func (d *genericDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Configure Type",
			fmt.Sprintf("Expected *pveclient.Client for %s, got: %T.", d.desc.Name, req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *genericDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	values := configValues(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", fmt.Sprintf("%s: provider client is not configured.", d.desc.Name))
		return
	}
	for _, op := range d.desc.Reads {
		if op.SkipIfMissingParam && !pathParamsReady(d.desc, op, values) {
			continue
		}
		if !skipValuesReady(op, values) {
			continue
		}
		payload, _, err := runOp(ctx, d.client, d.desc, op, values)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error Reading %s", d.desc.Name),
				fmt.Sprintf("reading %s (%s): %s", d.desc.Name, op.Path, err),
			)
			return
		}
		if _, err := applyReadResponse(d.desc, op, payload, values, false); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error Reading %s", d.desc.Name),
				fmt.Sprintf("reading %s (%s): %s", d.desc.Name, op.Path, err),
			)
			return
		}
	}
	id, err := joinID(d.desc, values)
	if err != nil || id == "" {
		id = d.desc.Name
	}
	values["id"] = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, objectValue(d.desc, values))...)
}
