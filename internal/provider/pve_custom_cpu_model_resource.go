// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// customCPUReportedModels enumerates the pin's closed `reported-model` enum
// of QEMU/KVM models a custom CPU model can report to guests.
var customCPUReportedModels = []string{
	"486", "a64fx", "athlon", "Broadwell", "Broadwell-IBRS", "Broadwell-noTSX",
	"Broadwell-noTSX-IBRS", "Cascadelake-Server", "Cascadelake-Server-noTSX", "Cascadelake-Server-v2", "Cascadelake-Server-v4", "Cascadelake-Server-v5",
	"ClearwaterForest", "ClearwaterForest-v2", "ClearwaterForest-v3", "Conroe", "Cooperlake", "Cooperlake-v2",
	"core2duo", "coreduo", "cortex-a35", "cortex-a53", "cortex-a55", "cortex-a57",
	"cortex-a710", "cortex-a72", "cortex-a76", "cortex-a78ae", "DiamondRapids", "EPYC",
	"EPYC-Genoa", "EPYC-Genoa-v2", "EPYC-IBPB", "EPYC-Milan", "EPYC-Milan-v2", "EPYC-Milan-v3",
	"EPYC-Rome", "EPYC-Rome-v2", "EPYC-Rome-v3", "EPYC-Rome-v4", "EPYC-Rome-v5", "EPYC-Turin",
	"EPYC-v3", "EPYC-v4", "EPYC-v5", "GraniteRapids", "GraniteRapids-v2", "GraniteRapids-v3",
	"GraniteRapids-v4", "GraniteRapids-v5", "Haswell", "Haswell-IBRS", "Haswell-noTSX", "Haswell-noTSX-IBRS",
	"host", "Icelake-Client", "Icelake-Client-noTSX", "Icelake-Server", "Icelake-Server-noTSX", "Icelake-Server-v3",
	"Icelake-Server-v4", "Icelake-Server-v5", "Icelake-Server-v6", "Icelake-Server-v7", "IvyBridge", "IvyBridge-IBRS",
	"KnightsMill", "kvm32", "kvm64", "max", "Nehalem", "Nehalem-IBRS",
	"neoverse-n1", "neoverse-n2", "neoverse-v1", "Opteron_G1", "Opteron_G2", "Opteron_G3",
	"Opteron_G4", "Opteron_G5", "Penryn", "pentium", "pentium2", "pentium3",
	"phenom", "qemu32", "qemu64", "SandyBridge", "SandyBridge-IBRS", "SapphireRapids",
	"SapphireRapids-v2", "SapphireRapids-v3", "SapphireRapids-v4", "SapphireRapids-v5", "SapphireRapids-v6", "SierraForest",
	"SierraForest-v2", "SierraForest-v3", "SierraForest-v4", "SierraForest-v5", "Skylake-Client", "Skylake-Client-IBRS",
	"Skylake-Client-noTSX-IBRS", "Skylake-Client-v4", "Skylake-Server", "Skylake-Server-IBRS", "Skylake-Server-noTSX-IBRS", "Skylake-Server-v4",
	"Skylake-Server-v5", "Westmere", "Westmere-IBRS",
}

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveCustomCPUModelResource{}
	_ resource.ResourceWithConfigure   = &pveCustomCPUModelResource{}
	_ resource.ResourceWithImportState = &pveCustomCPUModelResource{}
)

// NewPveCustomCPUModelResource returns the resource implementation.
func NewPveCustomCPUModelResource() resource.Resource {
	return &pveCustomCPUModelResource{}
}

// pveCustomCPUModelResource manages a custom CPU model definition
// (/cluster/qemu/custom-cpu-models). Guests reference the model as
// `custom-<name>`. Mutations are synchronous per the pin.
type pveCustomCPUModelResource struct {
	client *pveclient.Client
}

// pveCustomCPUModelResourceModel is the Terraform-facing shape.
type pveCustomCPUModelResourceModel struct {
	Name          types.String `tfsdk:"name"`
	ReportedModel types.String `tfsdk:"reported_model"`
	Flags         types.String `tfsdk:"flags"`
	GuestPhysBits types.Int64  `tfsdk:"guest_phys_bits"`
	Hidden        types.Bool   `tfsdk:"hidden"`
	HvVendorID    types.String `tfsdk:"hv_vendor_id"`
	Level         types.Int64  `tfsdk:"level"`
	PhysBits      types.String `tfsdk:"phys_bits"`
	Digest        types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveCustomCPUModelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCustomCpuModel
}

// Schema implements resource.Resource.
func (r *pveCustomCPUModelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom CPU model definition (`/cluster/qemu/custom-cpu-models`), a named baseline model plus overrides that VMs reference as `custom-<name>` (the pin's `cputype` parameter; the `custom-` prefix is optional in API calls). Mutations are synchronous per the pin.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the custom CPU model (PVE `pve-configid` format, at most 40 characters; the pin's `cputype` parameter, where the `custom-` prefix is optional). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"reported_model": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "CPU model and vendor to report to the guest. Must be a QEMU/KVM supported model; only custom CPU model definitions can change what the guest OS sees. Must be one of: `486`, `a64fx`, `athlon`, `Broadwell`, `Broadwell-IBRS`, `Broadwell-noTSX`, `Broadwell-noTSX-IBRS`, `Cascadelake-Server`, `Cascadelake-Server-noTSX`, `Cascadelake-Server-v2`, `Cascadelake-Server-v4`, `Cascadelake-Server-v5`, `ClearwaterForest`, `ClearwaterForest-v2`, `ClearwaterForest-v3`, `Conroe`, `Cooperlake`, `Cooperlake-v2`, `core2duo`, `coreduo`, `cortex-a35`, `cortex-a53`, `cortex-a55`, `cortex-a57`, `cortex-a710`, `cortex-a72`, `cortex-a76`, `cortex-a78ae`, `DiamondRapids`, `EPYC`, `EPYC-Genoa`, `EPYC-Genoa-v2`, `EPYC-IBPB`, `EPYC-Milan`, `EPYC-Milan-v2`, `EPYC-Milan-v3`, `EPYC-Rome`, `EPYC-Rome-v2`, `EPYC-Rome-v3`, `EPYC-Rome-v4`, `EPYC-Rome-v5`, `EPYC-Turin`, `EPYC-v3`, `EPYC-v4`, `EPYC-v5`, `GraniteRapids`, `GraniteRapids-v2`, `GraniteRapids-v3`, `GraniteRapids-v4`, `GraniteRapids-v5`, `Haswell`, `Haswell-IBRS`, `Haswell-noTSX`, `Haswell-noTSX-IBRS`, `host`, `Icelake-Client`, `Icelake-Client-noTSX`, `Icelake-Server`, `Icelake-Server-noTSX`, `Icelake-Server-v3`, `Icelake-Server-v4`, `Icelake-Server-v5`, `Icelake-Server-v6`, `Icelake-Server-v7`, `IvyBridge`, `IvyBridge-IBRS`, `KnightsMill`, `kvm32`, `kvm64`, `max`, `Nehalem`, `Nehalem-IBRS`, `neoverse-n1`, `neoverse-n2`, `neoverse-v1`, `Opteron_G1`, `Opteron_G2`, `Opteron_G3`, `Opteron_G4`, `Opteron_G5`, `Penryn`, `pentium`, `pentium2`, `pentium3`, `phenom`, `qemu32`, `qemu64`, `SandyBridge`, `SandyBridge-IBRS`, `SapphireRapids`, `SapphireRapids-v2`, `SapphireRapids-v3`, `SapphireRapids-v4`, `SapphireRapids-v5`, `SapphireRapids-v6`, `SierraForest`, `SierraForest-v2`, `SierraForest-v3`, `SierraForest-v4`, `SierraForest-v5`, `Skylake-Client`, `Skylake-Client-IBRS`, `Skylake-Client-noTSX-IBRS`, `Skylake-Client-v4`, `Skylake-Server`, `Skylake-Server-IBRS`, `Skylake-Server-noTSX-IBRS`, `Skylake-Server-v4`, `Skylake-Server-v5`, `Westmere`, `Westmere-IBRS`.",
				Validators: []validator.String{
					stringvalidator.OneOf(customCPUReportedModels...),
				},
			},
			"flags": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Additional CPU flags separated by `;`. Use `+FLAG` to enable and `-FLAG` to disable a flag. `nested-virt` is a shorthand controlling nested virtualization (`vmx` on Intel, `svm` on AMD). Custom models may use any QEMU/KVM flag; the VM-restricted set is: aes, amd-no-ssb, amd-ssbd, hv-evmcs, hv-tlbflush, ibpb, md-clear, nested-virt, pcid, pdpe1gb, spec-ctrl, ssbd, virt-ssbd.",
			},
			"guest_phys_bits": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Number of physical address bits available to the guest. Must be between 32 and 64.",
				Validators: []validator.Int64{
					int64validator.Between(32, 64),
				},
			},
			"hidden": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Do not identify as a KVM virtual machine; only affects x86-64 vCPUs. PVE defaults to false.",
			},
			"hv_vendor_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The Hyper-V vendor ID (1-12 alphanumeric characters); some Windows drivers or programs need a specific ID.",
			},
			"level": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum input value for the basic CPUID leaves the guest can query. `30` is a common workaround for Hyper-V boot failures on Windows guests on recent Intel hosts; only applies to x86-64 vCPUs. Must be between 0 and 4294967295.",
				Validators: []validator.Int64{
					int64validator.Between(0, 4294967295),
				},
			},
			"phys_bits": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Physical memory address bits reported to the guest OS: `8` to `64`, or `host` to use the host value (which breaks live migration to hosts with other values). Should be smaller than or equal to the host's.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the custom CPU model configuration; used for concurrent-modification detection.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveCustomCPUModelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveCustomCPUModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveCustomCPUModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_custom_cpu_model", "provider client is not configured")
		return
	}
	if err := r.client.CreateCustomCPUModel(ctx, customCPUModelFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_custom_cpu_model",
			fmt.Sprintf("creating custom CPU model %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_custom_cpu_model after create",
			fmt.Sprintf("reading custom CPU model %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveCustomCPUModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveCustomCPUModelResourceModel
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
			"Error reading pve_custom_cpu_model",
			fmt.Sprintf("reading custom CPU model %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveCustomCPUModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveCustomCPUModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveCustomCPUModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_custom_cpu_model", "provider client is not configured")
		return
	}
	deleteFields := customCPUModelDeleteFields(plan, state)
	if err := r.client.UpdateCustomCPUModel(ctx, plan.Name.ValueString(), customCPUModelFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_custom_cpu_model",
			fmt.Sprintf("updating custom CPU model %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_custom_cpu_model after update",
			fmt.Sprintf("reading custom CPU model %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveCustomCPUModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveCustomCPUModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCustomCPUModel(ctx, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_custom_cpu_model",
			fmt.Sprintf("deleting custom CPU model %s: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveCustomCPUModelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_custom_cpu_model import ID", "import ID must be the custom CPU model name, e.g. `lab-cpu`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveCustomCPUModelResource) readInto(ctx context.Context, m *pveCustomCPUModelResourceModel) error {
	model, err := r.client.GetCustomCPUModel(ctx, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.ReportedModel = nodeNetworkStringToTF(model.ReportedModel)
	m.Flags = nodeNetworkStringToTF(model.Flags)
	m.GuestPhysBits = haInt64PtrToTF(model.GuestPhysBits)
	m.Hidden = nodeNetworkBoolPtrToTF(model.Hidden)
	m.HvVendorID = nodeNetworkStringToTF(model.HVVendorID)
	m.Level = haInt64PtrToTF(model.Level)
	m.PhysBits = nodeNetworkStringToTF(model.PhysBits)
	m.Digest = nodeNetworkStringToTF(model.Digest)
	return nil
}

// customCPUModelFromModel projects the Terraform model into the wire body.
func customCPUModelFromModel(m pveCustomCPUModelResourceModel) pveclient.CustomCPUModel {
	body := pveclient.CustomCPUModel{Name: m.Name.ValueString()}
	if !m.ReportedModel.IsNull() && !m.ReportedModel.IsUnknown() {
		body.ReportedModel = m.ReportedModel.ValueString()
	}
	if !m.Flags.IsNull() && !m.Flags.IsUnknown() {
		body.Flags = m.Flags.ValueString()
	}
	if !m.GuestPhysBits.IsNull() && !m.GuestPhysBits.IsUnknown() {
		body.GuestPhysBits = pveclient.HAInt64Ptr(m.GuestPhysBits.ValueInt64())
	}
	if !m.Hidden.IsNull() && !m.Hidden.IsUnknown() {
		body.Hidden = pveclient.HABoolPtr(m.Hidden.ValueBool())
	}
	if !m.HvVendorID.IsNull() && !m.HvVendorID.IsUnknown() {
		body.HVVendorID = m.HvVendorID.ValueString()
	}
	if !m.Level.IsNull() && !m.Level.IsUnknown() {
		body.Level = pveclient.HAInt64Ptr(m.Level.ValueInt64())
	}
	if !m.PhysBits.IsNull() && !m.PhysBits.IsUnknown() {
		body.PhysBits = m.PhysBits.ValueString()
	}
	return body
}

// customCPUModelDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func customCPUModelDeleteFields(plan, state pveCustomCPUModelResourceModel) []string {
	var out []string
	if plan.Flags.IsNull() && !state.Flags.IsNull() {
		out = append(out, "flags")
	}
	if plan.GuestPhysBits.IsNull() && !state.GuestPhysBits.IsNull() {
		out = append(out, "guest-phys-bits")
	}
	if plan.Hidden.IsNull() && !state.Hidden.IsNull() {
		out = append(out, "hidden")
	}
	if plan.HvVendorID.IsNull() && !state.HvVendorID.IsNull() {
		out = append(out, "hv-vendor-id")
	}
	if plan.Level.IsNull() && !state.Level.IsNull() {
		out = append(out, "level")
	}
	if plan.PhysBits.IsNull() && !state.PhysBits.IsNull() {
		out = append(out, "phys-bits")
	}
	return out
}
