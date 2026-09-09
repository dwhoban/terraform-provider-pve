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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveHardwareMappingUsbResource{}
	_ resource.ResourceWithConfigure   = &pveHardwareMappingUsbResource{}
	_ resource.ResourceWithImportState = &pveHardwareMappingUsbResource{}
)

// NewPveHardwareMappingUsbResource returns the resource implementation.
func NewPveHardwareMappingUsbResource() resource.Resource {
	return &pveHardwareMappingUsbResource{}
}

// pveHardwareMappingUsbResource manages a logical USB hardware mapping
// (/cluster/mapping/usb). Updates send the full map per the pin's PUT
// semantics. Mutations are synchronous.
type pveHardwareMappingUsbResource struct {
	client *pveclient.Client
}

// pveHardwareMappingUsbResourceModel is the Terraform-facing shape.
type pveHardwareMappingUsbResourceModel struct {
	ID          types.String                      `tfsdk:"id"`
	Description types.String                      `tfsdk:"description"`
	Map         []pveHardwareMappingUsbEntryModel `tfsdk:"map"`
}

// pveHardwareMappingUsbEntryModel is one per-node device entry.
type pveHardwareMappingUsbEntryModel struct {
	Node        types.String `tfsdk:"node"`
	ID          types.String `tfsdk:"id"`
	Path        types.String `tfsdk:"path"`
	Description types.String `tfsdk:"description"`
}

// Metadata implements resource.Resource.
func (r *pveHardwareMappingUsbResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHardwareMappingUsb
}

// Schema implements resource.Resource.
func (r *pveHardwareMappingUsbResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a logical USB hardware mapping (`/cluster/mapping/usb`), declaring one entry per cluster node that carries the device. Updates replace the full entry list per the pin's PUT semantics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the logical USB mapping (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the logical USB device (up to 4096 characters).",
			},
			"map": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "Per-node device entries; one entry per node that carries the hardware.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Expected vendor and device ID in `vendor:device` form (e.g. `8087:0a2a`); with a `path` given it is only used for detecting hardware changes.",
						},
						"path": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "USB port path of the device in `<bus>-<port>` form (e.g. `1-2` or `1-2.3`).",
						},
						"description": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Description of the node-specific device (up to 4096 characters).",
						},
					},
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveHardwareMappingUsbResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveHardwareMappingUsbResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveHardwareMappingUsbResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_hardware_mapping_usb", "provider client is not configured")
		return
	}
	if err := r.client.CreateMappingUSB(ctx, hardwareMappingUsbFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_hardware_mapping_usb",
			fmt.Sprintf("creating USB mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_usb after create",
			fmt.Sprintf("reading USB mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveHardwareMappingUsbResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveHardwareMappingUsbResourceModel
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
			"Error reading pve_hardware_mapping_usb",
			fmt.Sprintf("reading USB mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. The PUT carries the full entry list,
// so only a removed description needs the delete parameter.
func (r *pveHardwareMappingUsbResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveHardwareMappingUsbResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveHardwareMappingUsbResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_hardware_mapping_usb", "provider client is not configured")
		return
	}
	deleteFields := hardwareMappingUsbDeleteFields(plan, state)
	if err := r.client.UpdateMappingUSB(ctx, plan.ID.ValueString(), hardwareMappingUsbFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_hardware_mapping_usb",
			fmt.Sprintf("updating USB mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_usb after update",
			fmt.Sprintf("reading USB mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveHardwareMappingUsbResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveHardwareMappingUsbResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMappingUSB(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_hardware_mapping_usb",
			fmt.Sprintf("deleting USB mapping %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveHardwareMappingUsbResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_hardware_mapping_usb import ID", "import ID must be the USB mapping identifier, e.g. `ups`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveHardwareMappingUsbResource) readInto(ctx context.Context, m *pveHardwareMappingUsbResourceModel) error {
	mapping, err := r.client.GetMappingUSB(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	m.Description = nodeNetworkStringToTF(mapping.Description)
	m.Map = make([]pveHardwareMappingUsbEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		m.Map = append(m.Map, pveHardwareMappingUsbEntryModel{
			Node:        types.StringValue(e.Node),
			ID:          nodeNetworkStringToTF(e.ID),
			Path:        nodeNetworkStringToTF(e.Path),
			Description: nodeNetworkStringToTF(e.Description),
		})
	}
	return nil
}

// hardwareMappingUsbFromModel projects the Terraform model into the wire
// body.
func hardwareMappingUsbFromModel(m pveHardwareMappingUsbResourceModel) pveclient.MappingUSB {
	body := pveclient.MappingUSB{ID: m.ID.ValueString()}
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		body.Description = m.Description.ValueString()
	}
	for _, e := range m.Map {
		entry := pveclient.MappingUSBEntry{Node: e.Node.ValueString()}
		if !e.ID.IsNull() && !e.ID.IsUnknown() {
			entry.ID = e.ID.ValueString()
		}
		if !e.Path.IsNull() && !e.Path.IsUnknown() {
			entry.Path = e.Path.ValueString()
		}
		if !e.Description.IsNull() && !e.Description.IsUnknown() {
			entry.Description = e.Description.ValueString()
		}
		body.Map = append(body.Map, entry)
	}
	return body
}

// hardwareMappingUsbDeleteFields returns the PVE field names to clear on
// update: optional attributes present in state but null in plan.
func hardwareMappingUsbDeleteFields(plan, state pveHardwareMappingUsbResourceModel) []string {
	var out []string
	if plan.Description.IsNull() && !state.Description.IsNull() {
		out = append(out, "description")
	}
	return out
}
