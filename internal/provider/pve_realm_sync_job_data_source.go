// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveRealmSyncJobDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRealmSyncJobDataSource{}
)

// NewPveRealmSyncJobDataSource returns the data source implementation.
func NewPveRealmSyncJobDataSource() datasource.DataSource {
	return &pveRealmSyncJobDataSource{}
}

// pveRealmSyncJobDataSource reads one realm-sync job definition from
// /cluster/jobs/realm-sync/{id}.
type pveRealmSyncJobDataSource struct {
	client *pveclient.Client
}

// pveRealmSyncJobDataSourceModel is the Terraform-facing shape.
type pveRealmSyncJobDataSourceModel struct {
	realmSyncJobModel
}

// Metadata implements datasource.DataSource.
func (d *pveRealmSyncJobDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmSyncJob
}

// Schema implements datasource.DataSource.
func (d *pveRealmSyncJobDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a realm-sync job definition (`/cluster/jobs/realm-sync/{id}`).",
		Attributes:          realmSyncJobDataSourceAttributes(),
	}
}

// realmSyncJobDataSourceAttributes returns the data-source attribute set:
// id stays Required as the query key, every other attribute is Computed.
func realmSyncJobDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := realmDataSourceAttributes(realmSyncJobAttributes(true, false))
	attrs["id"] = datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Job ID of the realm-sync job to read.",
		Validators: []validator.String{
			stringvalidator.LengthAtMost(64),
		},
	}
	return attrs
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRealmSyncJobDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRealmSyncJobDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveRealmSyncJobDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := config.ID.ValueString()
	job, err := d.client.GetRealmSyncJob(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_sync_job",
			fmt.Sprintf("reading job %s: %s", id, err),
		)
		return
	}
	realmSyncJobIntoModel(&config.realmSyncJobModel, job)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
