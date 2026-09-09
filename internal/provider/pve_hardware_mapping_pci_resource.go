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
	_ resource.Resource                = &pveHardwareMappingPciResource{}
	_ resource.ResourceWithConfigure   = &pveHardwareMappingPciResource{}
	_ resource.ResourceWithImportState = &pveHardwareMappingPciResource{}
)

// NewPveHardwareMappingPciResource returns the resource implementation.
func NewPveHardwareMappingPciResource() resource.Resource {
	return &pveHardwareMappingPciResource{}
}

// pveHardwareMappingPciResource manages a logical PCI hardware mapping
// (/cluster/mapping/pci). Updates send the full map per the pin's PUT
// semantics. Mutations are synchronous.
type pveHardwareMappingPciResource struct {
	client *pveclient.Client
}

// pveHardwareMappingPciResourceModel is the Terraform-facing shape.
type pveHardwareMappingPciResourceModel struct {
	ID                   types.String                      `tfsdk:"id"`
	Description          types.String                      `tfsdk:"description"`
	Mdev                 types.Bool                        `tfsdk:"mdev"`
	LiveMigrationCapable types.Bool                        `tfsdk:"live_migration_capable"`
	Map                  []pveHardwareMappingPciEntryModel `tfsdk:"map"`
}

// pveHardwareMappingPciEntryModel is one per-node device entry.
type pveHardwareMappingPciEntryModel struct {
	Node        types.String `tfsdk:"node"`
	ID          types.String `tfsdk:"id"`
	IommuGroup  types.Int64  `tfsdk:"iommugroup"`
	Path        types.String `tfsdk:"path"`
	SubsystemID types.String `tfsdk:"subsystem_id"`
	Description types.String `tfsdk:"description"`
}

// Metadata implements resource.Resource.
func (r *pveHardwareMappingPciResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHardwareMappingPci
}

// Schema implements resource.Resource.
func (r *pveHardwareMappingPciResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a logical PCI hardware mapping (`/cluster/mapping/pci`), declaring one entry per cluster node that carries the device. Updates replace the full entry list per the pin's PUT semantics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the logical PCI mapping (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the logical PCI device (up to 4096 characters).",
			},
			"mdev": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Marks the device(s) as capable of providing mediated devices. PVE defaults to false.",
			},
			"live_migration_capable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Marks the device(s) as able to be live-migrated (experimental; needs hardware and driver support). PVE defaults to false.",
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
							MarkdownDescription: "Expected vendor and device ID in `vendor:device` form (e.g. `10de:2231`), used for detecting hardware changes.",
						},
						"iommugroup": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "The IOMMU group the device is expected in, used for detecting hardware changes.",
						},
						"path": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "PCI path of the device (e.g. `0000:01:00.0`). Multiple paths may be given as a semicolon-separated list; the first available one is chosen on guest start. When omitted the whole device is mapped.",
						},
						"subsystem_id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Expected subsystem vendor and device ID in `vendor:device` form, used for detecting hardware changes.",
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
func (r *pveHardwareMappingPciResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveHardwareMappingPciResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveHardwareMappingPciResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_hardware_mapping_pci", "provider client is not configured")
		return
	}
	if err := r.client.CreateMappingPCI(ctx, hardwareMappingPciFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_hardware_mapping_pci",
			fmt.Sprintf("creating PCI mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_pci after create",
			fmt.Sprintf("reading PCI mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveHardwareMappingPciResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveHardwareMappingPciResourceModel
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
			"Error reading pve_hardware_mapping_pci",
			fmt.Sprintf("reading PCI mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. The PUT carries the full entry list,
// so only removed top-level optionals need the delete parameter.
func (r *pveHardwareMappingPciResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveHardwareMappingPciResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveHardwareMappingPciResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_hardware_mapping_pci", "provider client is not configured")
		return
	}
	deleteFields := hardwareMappingPciDeleteFields(plan, state)
	if err := r.client.UpdateMappingPCI(ctx, plan.ID.ValueString(), hardwareMappingPciFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_hardware_mapping_pci",
			fmt.Sprintf("updating PCI mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_pci after update",
			fmt.Sprintf("reading PCI mapping %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveHardwareMappingPciResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveHardwareMappingPciResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMappingPCI(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_hardware_mapping_pci",
			fmt.Sprintf("deleting PCI mapping %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveHardwareMappingPciResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_hardware_mapping_pci import ID", "import ID must be the PCI mapping identifier, e.g. `gpu`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveHardwareMappingPciResource) readInto(ctx context.Context, m *pveHardwareMappingPciResourceModel) error {
	mapping, err := r.client.GetMappingPCI(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	m.Description = nodeNetworkStringToTF(mapping.Description)
	m.Mdev = nodeNetworkBoolPtrToTF(mapping.Mdev)
	m.LiveMigrationCapable = nodeNetworkBoolPtrToTF(mapping.LiveMigrationCapable)
	m.Map = make([]pveHardwareMappingPciEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		m.Map = append(m.Map, pveHardwareMappingPciEntryModel{
			Node:        types.StringValue(e.Node),
			ID:          nodeNetworkStringToTF(e.ID),
			IommuGroup:  haInt64PtrToTF(e.IOMMUGroup),
			Path:        nodeNetworkStringToTF(e.Path),
			SubsystemID: nodeNetworkStringToTF(e.SubsystemID),
			Description: nodeNetworkStringToTF(e.Description),
		})
	}
	return nil
}

// hardwareMappingPciFromModel projects the Terraform model into the wire
// body.
func hardwareMappingPciFromModel(m pveHardwareMappingPciResourceModel) pveclient.MappingPCI {
	body := pveclient.MappingPCI{ID: m.ID.ValueString()}
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		body.Description = m.Description.ValueString()
	}
	if !m.Mdev.IsNull() && !m.Mdev.IsUnknown() {
		body.Mdev = pveclient.HABoolPtr(m.Mdev.ValueBool())
	}
	if !m.LiveMigrationCapable.IsNull() && !m.LiveMigrationCapable.IsUnknown() {
		body.LiveMigrationCapable = pveclient.HABoolPtr(m.LiveMigrationCapable.ValueBool())
	}
	for _, e := range m.Map {
		entry := pveclient.MappingPCIEntry{Node: e.Node.ValueString()}
		if !e.ID.IsNull() && !e.ID.IsUnknown() {
			entry.ID = e.ID.ValueString()
		}
		if !e.IommuGroup.IsNull() && !e.IommuGroup.IsUnknown() {
			entry.IOMMUGroup = pveclient.HAInt64Ptr(e.IommuGroup.ValueInt64())
		}
		if !e.Path.IsNull() && !e.Path.IsUnknown() {
			entry.Path = e.Path.ValueString()
		}
		if !e.SubsystemID.IsNull() && !e.SubsystemID.IsUnknown() {
			entry.SubsystemID = e.SubsystemID.ValueString()
		}
		if !e.Description.IsNull() && !e.Description.IsUnknown() {
			entry.Description = e.Description.ValueString()
		}
		body.Map = append(body.Map, entry)
	}
	return body
}

// hardwareMappingPciDeleteFields returns the PVE field names to clear on
// update: optional attributes present in state but null in plan.
func hardwareMappingPciDeleteFields(plan, state pveHardwareMappingPciResourceModel) []string {
	var out []string
	if plan.Description.IsNull() && !state.Description.IsNull() {
		out = append(out, "description")
	}
	if plan.Mdev.IsNull() && !state.Mdev.IsNull() {
		out = append(out, "mdev")
	}
	if plan.LiveMigrationCapable.IsNull() && !state.LiveMigrationCapable.IsNull() {
		out = append(out, "live-migration-capable")
	}
	return out
}
