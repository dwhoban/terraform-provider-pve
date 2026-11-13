// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// genericDataSource implements every generated data source: each declared
// read populates its ResultName attribute (or merges into top-level
// computed attributes) from one GET endpoint.
type genericDataSource struct {
	engineBase
}

var (
	_ datasource.DataSource              = (*genericDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*genericDataSource)(nil)
)

// NewGenericDataSource returns the data source constructor for a descriptor.
func NewGenericDataSource(d *TypeDescriptor) func() datasource.DataSource {
	return func() datasource.DataSource { return &genericDataSource{engineBase: engineBase{desc: d}} }
}

func (d *genericDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = d.desc.Name
}

func (d *genericDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = buildDataSourceSchema(d.desc)
}

func (d *genericDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if err := d.setClient(req.ProviderData); err != nil {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", err.Error())
	}
}

func (d *genericDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	values := configValues(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
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
			opError(&resp.Diagnostics, "Reading", d.desc, op.Path, err)
			return
		}
		// Data sources read server values wholesale into computed
		// attributes — the adopt semantics of import reads.
		if _, err := applyReadResponse(d.desc, op, payload, values, reconcileImport); err != nil {
			opError(&resp.Diagnostics, "Reading", d.desc, op.Path, err)
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
