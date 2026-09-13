// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveFileDataSource{}
	_ datasource.DataSourceWithConfigure = &pveFileDataSource{}
)

// NewPveFileDataSource returns the data source implementation.
func NewPveFileDataSource() datasource.DataSource {
	return &pveFileDataSource{}
}

// pveFileDataSource reads a single file from a PVE storage via the
// /nodes/{node}/storage/{storage}/content listing.
type pveFileDataSource struct {
	client *pveclient.Client
}

// pveFileDataSourceModel is the Terraform-facing shape.
type pveFileDataSourceModel struct {
	Node        types.String `tfsdk:"node"`
	Storage     types.String `tfsdk:"storage"`
	FileName    types.String `tfsdk:"file_name"`
	ContentType types.String `tfsdk:"content_type"`
	Volid       types.String `tfsdk:"volid"`
	Size        types.Int64  `tfsdk:"size"`
	Format      types.String `tfsdk:"format"`
	Ctime       types.Int64  `tfsdk:"ctime"`
	Notes       types.String `tfsdk:"notes"`
	ID          types.String `tfsdk:"id"`
}

// Metadata implements datasource.DataSource.
func (d *pveFileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFile
}

// Schema implements datasource.DataSource.
func (d *pveFileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single file (ISO image, container template or import image) from a PVE storage content listing, including its size, format and creation time.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node through which the storage is queried.",
			},
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier holding the file.",
			},
			"file_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the file on the storage, e.g. `debian-12.iso`. The volume is resolved by exact ID `<storage>:<content type>/<file name>`; PVE may normalize the name upstream, so use the name as it appears on the storage.",
			},
			"content_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage content type of the file. Must be one of: `iso`, `vztmpl`, `import`.",
				Validators: []validator.String{
					stringvalidator.OneOf("iso", "vztmpl", "import"),
				},
			},
			"volid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The full PVE volume ID of the file, e.g. `local:iso/debian-12.iso`.",
			},
			"size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "File size in bytes, as reported by the storage content listing; null when the storage does not report it.",
			},
			"format": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Format identifier of the volume, e.g. `iso`, `tgz` or `raw`.",
			},
			"ctime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Creation time of the file (seconds since the UNIX epoch); null when the storage does not report it.",
			},
			"notes": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Optional notes attached to the volume; only the first line of multi-line notes is returned by the listing.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The full PVE volume ID; identical to `volid`.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveFileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveFileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveFileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	want := storageFileVolid(data.Storage.ValueString(), data.ContentType.ValueString(), data.FileName.ValueString())
	entries, err := d.client.ListStorageContent(ctx, data.Node.ValueString(), data.Storage.ValueString(), data.ContentType.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_file data source",
			fmt.Sprintf("listing storage content on %s/%s: %s", data.Node.ValueString(), data.Storage.ValueString(), err),
		)
		return
	}
	entry, found := storageFileFindEntry(entries, want)
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_file data source",
			fmt.Sprintf("no %s content entry matches file %s on %s/%s", data.ContentType.ValueString(), data.FileName.ValueString(), data.Node.ValueString(), data.Storage.ValueString()),
		)
		return
	}
	data.Volid = types.StringValue(entry.Volid)
	data.Format = storageFileStringToTF(entry.Format)
	data.Size = storageFileInt64ToTF(entry.Size)
	data.Ctime = storageFileInt64ToTF(entry.Ctime)
	data.Notes = storageFileOptStringToTF(entry.Notes)
	data.ID = data.Volid
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
