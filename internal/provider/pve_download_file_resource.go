// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveDownloadFileResource{}
	_ resource.ResourceWithConfigure   = &pveDownloadFileResource{}
	_ resource.ResourceWithImportState = &pveDownloadFileResource{}
)

// NewPveDownloadFileResource returns the resource implementation.
func NewPveDownloadFileResource() resource.Resource {
	return &pveDownloadFileResource{}
}

// pveDownloadFileResource manages a file fetched by PVE itself from an HTTP
// URL via POST /nodes/{node}/storage/{storage}/download-url; the transfer
// runs as a PVE worker task and no bytes travel through Terraform.
type pveDownloadFileResource struct {
	client *pveclient.Client
}

// pveDownloadFileResourceModel is the Terraform-facing shape.
type pveDownloadFileResourceModel struct {
	Node               types.String `tfsdk:"node"`
	Storage            types.String `tfsdk:"storage"`
	URL                types.String `tfsdk:"url"`
	FileName           types.String `tfsdk:"file_name"`
	ContentType        types.String `tfsdk:"content_type"`
	Checksum           types.String `tfsdk:"checksum"`
	ChecksumAlgorithm  types.String `tfsdk:"checksum_algorithm"`
	Compression        types.String `tfsdk:"compression"`
	VerifyCertificates types.Bool   `tfsdk:"verify_certificates"`
	Volid              types.String `tfsdk:"volid"`
	Size               types.Int64  `tfsdk:"size"`
	Format             types.String `tfsdk:"format"`
	Ctime              types.Int64  `tfsdk:"ctime"`
	Notes              types.String `tfsdk:"notes"`
}

// Metadata implements resource.Resource.
func (r *pveDownloadFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveDownloadFile
}

// Schema implements resource.Resource.
func (r *pveDownloadFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a file on a PVE storage that PVE downloads itself from an HTTP(S) URL (`POST .../download-url`). " +
			"The download runs as a PVE worker task on the node — the machine running Terraform never transfers the file — and the task is awaited before the resource is considered created.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node that performs the download. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier that receives the file. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The HTTP or HTTPS URL to download the file from. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^https?://`), "must be an http:// or https:// URL"),
				},
			},
			"file_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the file to create on the storage. The volume is resolved by exact ID `<storage>:<content type>/<file name>`; PVE may normalize the name upstream, so use the name as it appears on the storage. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage content type of the file. Must be one of: `iso`, `vztmpl`, `import`. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("iso", "vztmpl", "import"),
				},
			},
			"checksum": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The expected checksum of the downloaded file; requires `checksum_algorithm`. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("checksum_algorithm")),
				},
			},
			"checksum_algorithm": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The algorithm used to verify `checksum`. Must be one of: `md5`, `sha1`, `sha224`, `sha256`, `sha384`, `sha512`. Requires `checksum`. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("md5", "sha1", "sha224", "sha256", "sha384", "sha512"),
					stringvalidator.AlsoRequires(path.MatchRoot("checksum")),
				},
			},
			"compression": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Decompress the downloaded file with the named compression algorithm before storing it (pass-through to PVE, e.g. `zst`); leave unset to keep the file exactly as downloaded. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"verify_certificates": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Verify the TLS certificates of the download server. Defaults to `true` upstream; set `false` only for sources with untrusted certificates. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
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
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveDownloadFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveDownloadFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveDownloadFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	opts := pveclient.StorageContentDownloadURLOptions{}
	if !plan.Checksum.IsNull() {
		checksum := plan.Checksum.ValueString()
		opts.Checksum = &checksum
	}
	if !plan.ChecksumAlgorithm.IsNull() {
		algorithm := plan.ChecksumAlgorithm.ValueString()
		opts.ChecksumAlgorithm = &algorithm
	}
	if !plan.Compression.IsNull() {
		compression := plan.Compression.ValueString()
		opts.Compression = &compression
	}
	if !plan.VerifyCertificates.IsNull() {
		verify := plan.VerifyCertificates.ValueBool()
		opts.VerifyCertificates = &verify
	}
	upid, err := r.client.DownloadStorageContentURL(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.URL.ValueString(), plan.FileName.ValueString(), plan.ContentType.ValueString(), &opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_download_file",
			fmt.Sprintf("starting download of %s to %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_download_file",
			fmt.Sprintf("waiting for download task of %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "downloaded PVE storage file from URL", map[string]any{"node": plan.Node.ValueString(), "storage": plan.Storage.ValueString(), "file_name": plan.FileName.ValueString()})
	plan.Volid = types.StringValue(storageFileVolid(plan.Storage.ValueString(), plan.ContentType.ValueString(), plan.FileName.ValueString()))
	found, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_download_file after create",
			fmt.Sprintf("reading %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_download_file after create",
			fmt.Sprintf("file %s did not appear in the %s/%s content listing after the download task finished; PVE may have normalized the file name upstream", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveDownloadFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveDownloadFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.readInto(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_download_file",
			fmt.Sprintf("reading %s on %s/%s: %s", state.FileName.ValueString(), state.Node.ValueString(), state.Storage.ValueString(), err),
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Every input attribute forces
// replacement, so updates only re-read the stored volume.
func (r *pveDownloadFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveDownloadFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_download_file after update",
			fmt.Sprintf("reading %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_download_file after update",
			fmt.Sprintf("file %s no longer exists on %s/%s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveDownloadFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveDownloadFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	volid := state.Volid.ValueString()
	if volid == "" {
		volid = storageFileVolid(state.Storage.ValueString(), state.ContentType.ValueString(), state.FileName.ValueString())
	}
	if err := r.client.DeleteStorageContent(ctx, state.Node.ValueString(), state.Storage.ValueString(), volid); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_download_file",
			fmt.Sprintf("deleting %s on %s/%s: %s", volid, state.Node.ValueString(), state.Storage.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE storage file", map[string]any{"node": state.Node.ValueString(), "storage": state.Storage.ValueString(), "volid": volid})
}

// ImportState parses an import ID of the form `<node>:<storage>:<volid>`,
// e.g. `pve1:local:local:iso/debian-12.iso`; the content type and file name
// are recovered from the volid and the follow-up read fills the rest.
func (r *pveDownloadFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, storage, contentType, fileName, err := storageFileParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("importing pve_download_file: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &pveDownloadFileResourceModel{
		Node:        types.StringValue(node),
		Storage:     types.StringValue(storage),
		ContentType: types.StringValue(contentType),
		FileName:    types.StringValue(fileName),
	})...)
}

// readInto refreshes the computed fields from the storage content listing
// and reports whether a matching volume still exists.
func (r *pveDownloadFileResource) readInto(ctx context.Context, m *pveDownloadFileResourceModel) (bool, error) {
	want := m.Volid.ValueString()
	if want == "" {
		want = storageFileVolid(m.Storage.ValueString(), m.ContentType.ValueString(), m.FileName.ValueString())
	}
	entries, err := r.client.ListStorageContent(ctx, m.Node.ValueString(), m.Storage.ValueString(), m.ContentType.ValueString())
	if err != nil {
		return false, fmt.Errorf("listing storage content: %w", err)
	}
	entry, found := storageFileFindEntry(entries, want)
	if !found {
		return false, nil
	}
	m.Volid = types.StringValue(entry.Volid)
	m.Format = storageFileStringToTF(entry.Format)
	m.Size = storageFileInt64ToTF(entry.Size)
	m.Ctime = storageFileInt64ToTF(entry.Ctime)
	m.Notes = storageFileOptStringToTF(entry.Notes)
	return true, nil
}
