// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveFileResource{}
	_ resource.ResourceWithConfigure   = &pveFileResource{}
	_ resource.ResourceWithImportState = &pveFileResource{}
)

// NewPveFileResource returns the resource implementation.
func NewPveFileResource() resource.Resource {
	return &pveFileResource{}
}

// pveFileResource manages a file on a PVE storage via the
// /nodes/{node}/storage/{storage} content endpoints: the upload comes from
// POST .../upload, reads from the content listing and deletes from
// DELETE .../content/{volid}.
type pveFileResource struct {
	client *pveclient.Client
}

// pveFileResourceModel is the Terraform-facing shape.
type pveFileResourceModel struct {
	Node        types.String `tfsdk:"node"`
	Storage     types.String `tfsdk:"storage"`
	FileName    types.String `tfsdk:"file_name"`
	ContentType types.String `tfsdk:"content_type"`
	Source      types.String `tfsdk:"source"`
	ContentB64  types.String `tfsdk:"content_base64"`
	Volid       types.String `tfsdk:"volid"`
	Size        types.Int64  `tfsdk:"size"`
	Format      types.String `tfsdk:"format"`
	Ctime       types.Int64  `tfsdk:"ctime"`
	Notes       types.String `tfsdk:"notes"`
}

// Metadata implements resource.Resource.
func (r *pveFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFile
}

// Schema implements resource.Resource.
func (r *pveFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a file (ISO image, container template or import image) on a PVE storage by uploading its bytes from the machine running Terraform. " +
			"The upload runs as a PVE worker task and is awaited before the resource is considered created.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node that performs the upload. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier that receives the file. Changing this value forces recreation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"file_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the file on the storage, e.g. `debian-12-standard_12.7-1_amd64.tar.zst`. The volume is resolved by exact ID `<storage>:<content type>/<file name>`; PVE may normalize the name upstream, so use the name as it appears on the storage. Changing this value forces recreation.",
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
			"source": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to the local file to upload, read from the machine running Terraform at apply time. Exactly one of `source` or `content_base64` must be set.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("source"), path.MatchRoot("content_base64")),
				},
			},
			"content_base64": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Base64-encoded contents of the file to upload. Exactly one of `source` or `content_base64` must be set.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("source"), path.MatchRoot("content_base64")),
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
func (r *pveFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, err := storageFilePayload(&plan.Source, &plan.ContentB64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_file",
			fmt.Sprintf("preparing upload of %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	upid, err := r.client.UploadStorageContent(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.FileName.ValueString(), plan.ContentType.ValueString(), data, nil, "")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_file",
			fmt.Sprintf("uploading %s to %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_file",
			fmt.Sprintf("waiting for upload task of %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "uploaded PVE storage file", map[string]any{"node": plan.Node.ValueString(), "storage": plan.Storage.ValueString(), "file_name": plan.FileName.ValueString()})
	plan.Volid = types.StringValue(storageFileVolid(plan.Storage.ValueString(), plan.ContentType.ValueString(), plan.FileName.ValueString()))
	found, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_file after create",
			fmt.Sprintf("reading %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_file after create",
			fmt.Sprintf("file %s did not appear in the %s/%s content listing after the upload task finished; PVE may have normalized the file name upstream", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.readInto(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_file",
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
func (r *pveFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_file after update",
			fmt.Sprintf("reading %s on %s/%s: %s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString(), err),
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_file after update",
			fmt.Sprintf("file %s no longer exists on %s/%s", plan.FileName.ValueString(), plan.Node.ValueString(), plan.Storage.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveFileResourceModel
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
			"Error deleting pve_file",
			fmt.Sprintf("deleting %s on %s/%s: %s", volid, state.Node.ValueString(), state.Storage.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE storage file", map[string]any{"node": state.Node.ValueString(), "storage": state.Storage.ValueString(), "volid": volid})
}

// ImportState parses an import ID of the form `<node>:<storage>:<volid>`,
// e.g. `pve1:local:local:iso/debian-12.iso`; the content type and file name
// are recovered from the volid and the follow-up read fills the rest.
func (r *pveFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, storage, contentType, fileName, err := storageFileParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("importing pve_file: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &pveFileResourceModel{
		Node:        types.StringValue(node),
		Storage:     types.StringValue(storage),
		ContentType: types.StringValue(contentType),
		FileName:    types.StringValue(fileName),
	})...)
}

// readInto refreshes the computed fields from the storage content listing
// and reports whether a matching volume still exists.
func (r *pveFileResource) readInto(ctx context.Context, m *pveFileResourceModel) (bool, error) {
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
	storageFileApplyEntry(m, entry.Volid, entry.Format, entry.Size, entry.Ctime, entry.Notes)
	return true, nil
}

// storageFilePayload resolves the file bytes from exactly one of the two
// source attributes; the schema validator guarantees one is set.
func storageFilePayload(source, contentB64 *types.String) ([]byte, error) {
	if source != nil && !source.IsNull() {
		return os.ReadFile(source.ValueString())
	}
	if contentB64 != nil && !contentB64.IsNull() {
		return base64.StdEncoding.DecodeString(contentB64.ValueString())
	}
	return nil, fmt.Errorf("exactly one of source or content_base64 must be set")
}

// storageFileVolid builds the PVE volume ID of an uploaded file:
// `<storage>:<content type>/<file name>`.
func storageFileVolid(storage, contentType, fileName string) string {
	return storage + ":" + contentType + "/" + fileName
}

// storageFileSplitVolid splits a volume ID into its content type and file
// name; the second return is false when the ID does not follow the
// `<storage>:<type>/<name>` shape.
func storageFileSplitVolid(volid string) (string, string, bool) {
	colon := strings.Index(volid, ":")
	if colon < 0 {
		return "", "", false
	}
	rest := volid[colon+1:]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

// storageFileFindEntry locates a volume in a content listing by exact
// volume ID. PVE normalizes uploaded file names upstream; when the stored
// name differs from the requested one the lookup misses and the caller
// surfaces that as an error instead of silently adopting a renamed volume.
func storageFileFindEntry(entries []pveclient.NodeStorageContentFile, wantVolid string) (pveclient.NodeStorageContentFile, bool) {
	for _, entry := range entries {
		if entry.Volid == wantVolid {
			return entry, true
		}
	}
	return pveclient.NodeStorageContentFile{}, false
}

// storageFileApplyEntry copies listing fields into a model's computed
// attributes; absent optional fields stay null so they do not churn.
func storageFileApplyEntry(m *pveFileResourceModel, volid, format string, size, ctime *int64, notes *string) {
	m.Volid = types.StringValue(volid)
	m.Format = storageFileStringToTF(format)
	m.Size = storageFileInt64ToTF(size)
	m.Ctime = storageFileInt64ToTF(ctime)
	m.Notes = storageFileOptStringToTF(notes)
}

// storageFileParseImportID splits `<node>:<storage>:<volid>` and recovers
// the content type and file name from the volid.
func storageFileParseImportID(id string) (node, storage, contentType, fileName string, err error) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", "", fmt.Errorf("expected import ID of the form <node>:<storage>:<volid> (e.g. pve1:local:local:iso/debian-12.iso), got %q", id)
	}
	contentType, fileName, ok := storageFileSplitVolid(parts[2])
	if !ok {
		return "", "", "", "", fmt.Errorf("volid %q in import ID does not follow the <storage>:<type>/<name> form", parts[2])
	}
	return parts[0], parts[1], contentType, fileName, nil
}

// storageFileStringToTF maps a required listing string to a Terraform
// string; the empty string becomes null so absent formats do not churn.
func storageFileStringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// storageFileOptStringToTF maps an optional listing string to a nullable
// Terraform string.
func storageFileOptStringToTF(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// storageFileInt64ToTF maps an optional listing integer to a nullable
// Terraform integer.
func storageFileInt64ToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
