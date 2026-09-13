// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveMappingDirResource{}
	_ resource.ResourceWithConfigure   = &pveMappingDirResource{}
	_ resource.ResourceWithImportState = &pveMappingDirResource{}
)

// NewPveMappingDirResource returns the resource implementation.
func NewPveMappingDirResource() resource.Resource {
	return &pveMappingDirResource{}
}

// pveMappingDirResource manages a directory hardware mapping
// (/cluster/mapping/dir), the cluster-side declaration of per-node
// directories shareable with guests. Updates send the full map per the
// pin's PUT semantics. Mutations are synchronous.
type pveMappingDirResource struct {
	client *pveclient.Client
}

// pveMappingDirResourceModel is the Terraform-facing shape.
type pveMappingDirResourceModel struct {
	ID          types.String              `tfsdk:"id"`
	Description types.String              `tfsdk:"description"`
	Map         []pveMappingDirEntryModel `tfsdk:"map"`
}

// pveMappingDirEntryModel is one per-node directory entry.
type pveMappingDirEntryModel struct {
	Node types.String `tfsdk:"node"`
	Path types.String `tfsdk:"path"`
}

// Metadata implements resource.Resource.
func (r *pveMappingDirResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveMappingDir
}

// Schema implements resource.Resource.
func (r *pveMappingDirResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a directory hardware mapping (`/cluster/mapping/dir`), declaring per-node directories that guests can consume (e.g. as VirtIOFS shares). Updates replace the full entry list per the pin's PUT semantics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the directory mapping (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the directory mapping (up to 4096 characters).",
			},
			"map": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "Per-node directory entries; one entry per node that carries the directory.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"path": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Absolute directory path on the node that should be shared with guests (e.g. `/mnt/share`).",
						},
					},
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveMappingDirResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveMappingDirResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveMappingDirResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_mapping_dir", "provider client is not configured")
		return
	}
	if err := r.client.CreateMappingDir(ctx, mappingDirFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_mapping_dir",
			fmt.Sprintf("creating directory mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_mapping_dir after create",
			fmt.Sprintf("reading directory mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveMappingDirResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveMappingDirResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_mapping_dir",
			fmt.Sprintf("reading directory mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveMappingDirResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveMappingDirResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveMappingDirResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_mapping_dir", "provider client is not configured")
		return
	}
	deleteFields := mappingDirDeleteFields(plan, state)
	if err := r.client.UpdateMappingDir(ctx, plan.ID.ValueString(), mappingDirFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_mapping_dir",
			fmt.Sprintf("updating directory mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_mapping_dir after update",
			fmt.Sprintf("reading directory mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveMappingDirResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveMappingDirResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMappingDir(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_mapping_dir",
			fmt.Sprintf("deleting directory mapping %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveMappingDirResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_mapping_dir import ID", "import ID must be the directory mapping identifier, e.g. `share`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveMappingDirResource) readInto(ctx context.Context, m *pveMappingDirResourceModel) error {
	mapping, err := r.client.GetMappingDir(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	m.Description = nodeNetworkStringToTF(mapping.Description)
	m.Map = make([]pveMappingDirEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		m.Map = append(m.Map, pveMappingDirEntryModel{
			Node: types.StringValue(e.Node),
			Path: types.StringValue(e.Path),
		})
	}
	return nil
}

// mappingDirFromModel projects the Terraform model into the wire body.
func mappingDirFromModel(m pveMappingDirResourceModel) pveclient.MappingDir {
	body := pveclient.MappingDir{ID: m.ID.ValueString()}
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		body.Description = m.Description.ValueString()
	}
	for _, e := range m.Map {
		body.Map = append(body.Map, pveclient.MappingDirEntry{
			Node: e.Node.ValueString(),
			Path: e.Path.ValueString(),
		})
	}
	return body
}

// mappingDirDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func mappingDirDeleteFields(plan, state pveMappingDirResourceModel) []string {
	var out []string
	if plan.Description.IsNull() && !state.Description.IsNull() {
		out = append(out, "description")
	}
	return out
}
