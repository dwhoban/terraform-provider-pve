// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveClusterFirewallOptionsResource{}
	_ resource.ResourceWithConfigure   = &pveClusterFirewallOptionsResource{}
	_ resource.ResourceWithImportState = &pveClusterFirewallOptionsResource{}
)

// pveClusterFirewallOptionsID is the static identifier of the cluster
// firewall options singleton.
const pveClusterFirewallOptionsID = "cluster"

// firewallOptionsPolicyInValues is the closed input/output policy set of
// the pin.
var firewallOptionsPolicyInValues = []string{"ACCEPT", "REJECT", "DROP"}

// firewallOptionsPolicyForwardValues is the closed forward policy set of
// the pin (cluster and vnet scopes; no REJECT).
var firewallOptionsPolicyForwardValues = []string{"ACCEPT", "DROP"}

// firewallOptionsLogLevelValues is the closed log level set of the pin.
var firewallOptionsLogLevelValues = []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug", "nolog"}

// firewallOptionsEnumText renders the inline enumeration required in every
// closed-set attribute description.
func firewallOptionsEnumText(values []string) string {
	out := "Must be one of: "
	for i, v := range values {
		if i > 0 {
			out += ", "
		}
		out += "`" + v + "`"
	}
	return out + "."
}

// firewallOptionsStringValue writes a wire string into a model attribute,
// null when absent.
func firewallOptionsStringValue(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// firewallOptionsStrPtr converts a Terraform string into a wire pointer,
// nil for null or unknown values.
func firewallOptionsStrPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// firewallOptionsBoolPtr converts a Terraform bool into a wire pointer,
// nil for null or unknown values.
func firewallOptionsBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// firewallOptionsInt64Ptr converts a Terraform int64 into a wire pointer,
// nil for null or unknown values.
func firewallOptionsInt64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// firewallOptionsInt64Value writes a wire int into a model attribute, null
// when absent.
func firewallOptionsInt64Value(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

// NewPveClusterFirewallOptionsResource returns the resource implementation.
func NewPveClusterFirewallOptionsResource() resource.Resource {
	return &pveClusterFirewallOptionsResource{}
}

// pveClusterFirewallOptionsResource manages the cluster-wide firewall
// options singleton via GET/PUT /cluster/firewall/options.
type pveClusterFirewallOptionsResource struct {
	client *pveclient.Client
}

// pveClusterFirewallOptionsOptionSet carries the option fields shared by
// the resource and data source models; both embed it with its tfsdk tags.
type pveClusterFirewallOptionsOptionSet struct {
	Ebitables     types.Bool   `tfsdk:"ebtables"`
	Enable        types.Int64  `tfsdk:"enable"`
	LogRateLimit  types.String `tfsdk:"log_ratelimit"`
	PolicyForward types.String `tfsdk:"policy_forward"`
	PolicyIn      types.String `tfsdk:"policy_in"`
	PolicyOut     types.String `tfsdk:"policy_out"`
}

// pveClusterFirewallOptionsResourceModel is the Terraform-facing shape of
// the resource.
type pveClusterFirewallOptionsResourceModel struct {
	pveClusterFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// clusterFirewallOptionsFieldSpecs is the /cluster/firewall/options
// attribute set, in wire order, transcribed from the api-spec pin
// (GET/PUT /cluster/firewall/options).
var clusterFirewallOptionsFieldSpecs = []clusterOptionsField{
	{Name: "ebtables", Wire: "ebtables", Kind: clusterOptionsKindBool, Description: "Enable ebtables rules cluster wide."},
	{Name: "enable", Wire: "enable", Kind: clusterOptionsKindInt64, Description: "Enable or disable the firewall cluster wide.", Min: clusterOptionsF64(0)},
	{Name: "log_ratelimit", Wire: "log_ratelimit", Kind: clusterOptionsKindString, Description: "Log ratelimiting settings as a PVE property string: `[enable=]<1|0> [,burst=<integer>] [,rate=<rate>]`, e.g. `enable=1,burst=5,rate=1/second`."},
	{Name: "policy_forward", Wire: "policy_forward", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyForwardValues, Description: "Forward policy."},
	{Name: "policy_in", Wire: "policy_in", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyInValues, Description: "Input policy."},
	{Name: "policy_out", Wire: "policy_out", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyInValues, Description: "Output policy."},
}

// clusterFirewallOptionsResourceAttributes renders the full resource
// attribute set.
func clusterFirewallOptionsResourceAttributes() map[string]schema.Attribute {
	attrs := make(map[string]schema.Attribute, len(clusterFirewallOptionsFieldSpecs)+1)
	for _, f := range clusterFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsResourceLeaf(f)
	}
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Singleton identifier for the cluster firewall options; always `cluster`.",
	}
	return attrs
}

// Metadata implements resource.Resource.
func (r *pveClusterFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterFirewallOptions
}

// Schema implements resource.Resource.
func (r *pveClusterFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the cluster-wide firewall options singleton (`GET/PUT /cluster/firewall/options`). Every listed attribute is managed: removing an attribute from configuration clears the option on the cluster via the `delete` parameter. The singleton has no upstream delete verb, so destroy only forgets the state. Requires `Sys.Modify` on `/`.",
		Attributes:          clusterFirewallOptionsResourceAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveClusterFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create implements resource.Resource. A singleton has no upstream create
// verb; writing the planned options is the whole operation.
func (r *pveClusterFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveClusterFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_cluster_firewall_options", "provider client is not configured")
		return
	}
	if err := r.client.UpdateClusterFirewallOptions(ctx, clusterFirewallOptionsFromModel(&plan.pveClusterFirewallOptionsOptionSet), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_cluster_firewall_options",
			fmt.Sprintf("writing cluster firewall options: %s", err),
		)
		return
	}
	if err := clusterFirewallOptionsReadInto(ctx, r.client, &plan.pveClusterFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_firewall_options after create",
			fmt.Sprintf("reading cluster firewall options: %s", err),
		)
		return
	}
	plan.ID = types.StringValue(pveClusterFirewallOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveClusterFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveClusterFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := clusterFirewallOptionsReadInto(ctx, r.client, &state.pveClusterFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_firewall_options",
			fmt.Sprintf("reading cluster firewall options: %s", err),
		)
		return
	}
	state.ID = types.StringValue(pveClusterFirewallOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// fields cleared in the plan travel in the `delete` query parameter.
func (r *pveClusterFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveClusterFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveClusterFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := clusterFirewallOptionsDeleteFields(&plan.pveClusterFirewallOptionsOptionSet, &state.pveClusterFirewallOptionsOptionSet)
	if err := r.client.UpdateClusterFirewallOptions(ctx, clusterFirewallOptionsFromModel(&plan.pveClusterFirewallOptionsOptionSet), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_cluster_firewall_options",
			fmt.Sprintf("updating cluster firewall options: %s", err),
		)
		return
	}
	if err := clusterFirewallOptionsReadInto(ctx, r.client, &plan.pveClusterFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_firewall_options after update",
			fmt.Sprintf("reading cluster firewall options: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The firewall options singleton has
// no upstream delete verb in the pin, so destroy only forgets the state.
func (r *pveClusterFirewallOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState adopts the singleton; the identifier is always "cluster",
// whatever ID the import statement supplied.
func (r *pveClusterFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), pveClusterFirewallOptionsID)...)
}

// clusterFirewallOptionsReadInto refreshes the shared option set from the
// cluster.
func clusterFirewallOptionsReadInto(ctx context.Context, client *pveclient.Client, m *pveClusterFirewallOptionsOptionSet) error {
	opts, err := client.GetClusterFirewallOptions(ctx)
	if err != nil {
		return fmt.Errorf("reading cluster firewall options: %w", err)
	}
	clusterFirewallOptionsApply(m, opts)
	return nil
}

// clusterFirewallOptionsFromModel projects the Terraform model into the
// wire struct; null values become nil pointers and are omitted from the PUT
// body.
func clusterFirewallOptionsFromModel(m *pveClusterFirewallOptionsOptionSet) pveclient.ClusterFirewallOptions {
	return pveclient.ClusterFirewallOptions{
		Ebitables:     firewallOptionsBoolPtr(m.Ebitables),
		Enable:        firewallOptionsInt64Ptr(m.Enable),
		LogRateLimit:  firewallOptionsStrPtr(m.LogRateLimit),
		PolicyForward: firewallOptionsStrPtr(m.PolicyForward),
		PolicyIn:      firewallOptionsStrPtr(m.PolicyIn),
		PolicyOut:     firewallOptionsStrPtr(m.PolicyOut),
	}
}

// clusterFirewallOptionsApply writes fetched options into the model; absent
// options become null.
func clusterFirewallOptionsApply(m *pveClusterFirewallOptionsOptionSet, o *pveclient.ClusterFirewallOptions) {
	m.Ebitables = nodeNetworkBoolPtrToTF(o.Ebitables)
	m.Enable = firewallOptionsInt64Value(o.Enable)
	m.LogRateLimit = firewallOptionsStringValue(o.LogRateLimit)
	m.PolicyForward = firewallOptionsStringValue(o.PolicyForward)
	m.PolicyIn = firewallOptionsStringValue(o.PolicyIn)
	m.PolicyOut = firewallOptionsStringValue(o.PolicyOut)
}

// clusterFirewallOptionsDeleteFields returns the wire names of options
// present in state but cleared in the plan.
func clusterFirewallOptionsDeleteFields(plan, state *pveClusterFirewallOptionsOptionSet) []string {
	var out []string
	if plan.Ebitables.IsNull() && !state.Ebitables.IsNull() {
		out = append(out, "ebtables")
	}
	if plan.Enable.IsNull() && !state.Enable.IsNull() {
		out = append(out, "enable")
	}
	if plan.LogRateLimit.IsNull() && !state.LogRateLimit.IsNull() {
		out = append(out, "log_ratelimit")
	}
	if plan.PolicyForward.IsNull() && !state.PolicyForward.IsNull() {
		out = append(out, "policy_forward")
	}
	if plan.PolicyIn.IsNull() && !state.PolicyIn.IsNull() {
		out = append(out, "policy_in")
	}
	if plan.PolicyOut.IsNull() && !state.PolicyOut.IsNull() {
		out = append(out, "policy_out")
	}
	return out
}
