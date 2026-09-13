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
	_ datasource.DataSource              = &pveStorageFilesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageFilesDataSource{}
)

// NewPveStorageFilesDataSource returns the data source implementation.
func NewPveStorageFilesDataSource() datasource.DataSource {
	return &pveStorageFilesDataSource{}
}

// pveStorageFilesDataSource lists the files (volumes) stored on one storage
// (GET /nodes/{node}/storage/{storage}/content).
type pveStorageFilesDataSource struct {
	client *pveclient.Client
}

// pveStorageFilesDataSourceModel is the Terraform-facing shape.
type pveStorageFilesDataSourceModel struct {
	ID      types.String                         `tfsdk:"id"`
	Node    types.String                         `tfsdk:"node"`
	Storage types.String                         `tfsdk:"storage"`
	Content types.String                         `tfsdk:"content"`
	Files   []pveStorageFilesDataSourceFileModel `tfsdk:"files"`
}

// pveStorageFilesDataSourceFileModel mirrors one content row. Field names
// line up with the pveclient struct.
type pveStorageFilesDataSourceFileModel struct {
	Volid           types.String                                `tfsdk:"volid"`
	Format          types.String                                `tfsdk:"format"`
	Size            types.Int64                                 `tfsdk:"size"`
	ApproximateSize types.Int64                                 `tfsdk:"approximate_size"`
	Used            types.Int64                                 `tfsdk:"used"`
	VMID            types.Int64                                 `tfsdk:"vmid"`
	Notes           types.String                                `tfsdk:"notes"`
	Ctime           types.Int64                                 `tfsdk:"ctime"`
	Parent          types.String                                `tfsdk:"parent"`
	Protected       types.Bool                                  `tfsdk:"protected"`
	Encrypted       types.String                                `tfsdk:"encrypted"`
	Verification    *pveStorageFilesDataSourceVerificationModel `tfsdk:"verification"`
}

// pveStorageFilesDataSourceVerificationModel mirrors the nested PBS backup
// verification object.
type pveStorageFilesDataSourceVerificationModel struct {
	State types.String `tfsdk:"state"`
	Upid  types.String `tfsdk:"upid"`
}

// Metadata implements datasource.DataSource.
func (d *pveStorageFilesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageFiles
}

// Schema implements datasource.DataSource.
func (d *pveStorageFilesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the files stored on a storage, as reported by `GET /nodes/{node}/storage/{storage}/content`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in `node:storage` form.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name through which to address the storage.",
			},
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier whose content to list.",
			},
			"content": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only list content of this type. One of the PVE content types: `images`, `rootdir`, `vztmpl`, `iso`, `backup`, `snippets`, `import`. PVE filters the list upstream.",
			},
			"files": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Files on the storage.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"volid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Volume identifier (e.g. `local:iso/debian-12.iso`).",
						},
						"format": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Format identifier (`raw`, `qcow2`, `subvol`, `iso`, `tgz`, ...).",
						},
						"size": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Volume size in bytes; null where the storage cannot determine an exact size and reports `approximate_size` instead.",
						},
						"approximate_size": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Approximate volume size in bytes, present instead of `size` for storages with technical limits on exact size; null when the API reports exact `size`.",
						},
						"used": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Used space in bytes (most storage plugins report nothing useful here); null when the API did not report it.",
						},
						"vmid": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Associated owner VMID; null for unowned files.",
						},
						"notes": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Optional notes (first line only when multi-line); null when unset.",
						},
						"ctime": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Creation time in seconds since the UNIX epoch; null when the API did not report it.",
						},
						"parent": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Volume identifier of the parent (for linked clones); null when unset.",
						},
						"protected": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Protection status (currently only for backups); null when unset.",
						},
						"encrypted": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "For encrypted PBS backups, the fingerprint or `1`; null when unencrypted or not a backup.",
						},
						"verification": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Last backup verification result, only useful for PBS storages; null when none was reported.",
							Attributes: map[string]schema.Attribute{
								"state": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "Last backup verification state.",
								},
								"upid": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "Last backup verification UPID.",
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageFilesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveStorageFilesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageFilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	files, err := d.client.ListStorageContent(ctx, data.Node.ValueString(), data.Storage.ValueString(), data.Content.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_storage_files", fmt.Sprintf("listing content of storage %q on node %q: %s", data.Storage.ValueString(), data.Node.ValueString(), err))
		return
	}
	rows := make([]pveStorageFilesDataSourceFileModel, 0, len(files))
	for _, f := range files {
		rows = append(rows, pveStorageFilesDataSourceFileModel{
			Volid:           types.StringValue(f.Volid),
			Format:          types.StringValue(f.Format),
			Size:            nodeStoragesInt64ToTF(f.Size),
			ApproximateSize: nodeStoragesInt64ToTF(f.ApproximateSize),
			Used:            nodeStoragesInt64ToTF(f.Used),
			VMID:            nodeStoragesInt64ToTF(f.VMID),
			Notes:           nodeStoragesStringToTF(f.Notes),
			Ctime:           nodeStoragesInt64ToTF(f.Ctime),
			Parent:          nodeStoragesStringToTF(f.Parent),
			Protected:       nodeStoragesBoolToTF(f.Protected),
			Encrypted:       nodeStoragesStringToTF(f.Encrypted),
			Verification:    storageFilesVerificationToTF(f.Verification),
		})
	}
	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Node.ValueString(), data.Storage.ValueString()))
	data.Files = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// nodeStoragesStringToTF maps an optional API string onto a nullable TF
// string.
func nodeStoragesStringToTF(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// storageFilesVerificationToTF maps the optional nested verification object
// onto its model, keeping nil for absent.
func storageFilesVerificationToTF(v *pveclient.NodeStorageContentVerification) *pveStorageFilesDataSourceVerificationModel {
	if v == nil {
		return nil
	}
	m := &pveStorageFilesDataSourceVerificationModel{
		State: types.StringNull(),
		Upid:  types.StringNull(),
	}
	if v.State != nil {
		m.State = types.StringValue(*v.State)
	}
	if v.Upid != nil {
		m.Upid = types.StringValue(*v.Upid)
	}
	return m
}
