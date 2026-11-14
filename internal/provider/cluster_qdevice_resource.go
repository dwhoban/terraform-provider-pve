// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pvecmBinary is the absolute path granted in sudoers.
const pvecmBinary = "/usr/bin/pvecm"

var (
	qdeviceAddressRe = regexp.MustCompile(`^[0-9A-Fa-f.:]+$`)
	qdeviceNetworkRe = regexp.MustCompile(`^[0-9A-Fa-f.:]+/[0-9]{1,3}$`)
)

// clusterQDeviceResource configures a corosync QDevice with
// `pvecm qdevice setup`. The PVE API only reports QDevice status
// (GET /cluster/config/qdevice), so setup and removal run over SSH.
type clusterQDeviceResource struct {
	client *pveclient.Client
}

var (
	_ resource.Resource                = (*clusterQDeviceResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterQDeviceResource)(nil)
	_ resource.ResourceWithImportState = (*clusterQDeviceResource)(nil)
)

// NewClusterQDeviceResource is the resource constructor.
func NewClusterQDeviceResource() resource.Resource { return &clusterQDeviceResource{} }

type clusterQDeviceModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	Address   types.String `tfsdk:"address"`
	Network   types.String `tfsdk:"network"`
	Force     types.Bool   `tfsdk:"force"`
	QNetdHost types.String `tfsdk:"qnetd_host"`
	State     types.String `tfsdk:"state"`
	Algorithm types.String `tfsdk:"algorithm"`
	Model     types.String `tfsdk:"model"`
	TieBreak  types.String `tfsdk:"tie_breaker"`
}

// qdeviceStatus is the parsed GET /cluster/config/qdevice answer.
type qdeviceStatus struct {
	QNetdHost string // raw "QNetd host" value, e.g. 10.0.0.50:5403
	Host      string // QNetd host without the port
	State     string
	Algorithm string
	Model     string
	TieBreak  string
}

func (r *clusterQDeviceResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "pve_cluster_qdevice"
}

func (r *clusterQDeviceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	computed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Corosync QDevice of the cluster (`pvecm qdevice setup`). **Requires the provider's `ssh` block**: the Proxmox VE API can only report QDevice status, so this resource runs `/usr/bin/pvecm qdevice setup` and `remove` on `node` over SSH (as `root`, or through `sudo -n` — see the SSH guide for the exact sudoers rule). `pvecm` needs `corosync-qnetd` on the QNet host, `corosync-qdevice` on every node, all nodes online, and root SSH access from `node` to the QNet host. Destroying the resource runs `pvecm qdevice remove`.\n\nImport with `<node>/<address>`, for example `pve01/10.0.0.50`. `network` and `force` are not recoverable from the cluster, so they stay unset after an import; omit them in the configuration or accept the replacement they trigger.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The QNet daemon address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster member that runs `pvecm`.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(nodeNameRe, "must be a valid node name")},
			},
			"address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IP address of the host running `corosync-qnetd`. `pvecm` accepts only an IP address, not a hostname.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(qdeviceAddressRe, "must be an IPv4 or IPv6 address")},
			},
			"network": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "CIDR of the network `corosync-qdevice` uses to reach the QNet daemon (`--network`).",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(qdeviceNetworkRe, "must be a CIDR such as 10.0.0.0/24")},
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Pass `--force`. `pvecm` refuses to set up a QDevice for a cluster with an odd node count without it.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"qnetd_host":  computed("QNet daemon endpoint (`host:port`) the QDevice reports."),
			"state":       computed("QDevice connection state, for example `Connected`."),
			"algorithm":   computed("Decision algorithm in use, for example `Fifty-Fifty split`."),
			"model":       computed("QDevice model, for example `Net`."),
			"tie_breaker": computed("Tie-breaker setting reported by the QDevice."),
		},
	}
}

func (r *clusterQDeviceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", fmt.Sprintf("Expected *pveclient.Client for pve_cluster_qdevice, got: %T.", req.ProviderData))
		return
	}
	r.client = c
}

// qdeviceSetupArgv builds the exact remote command; values are validated by
// the schema, and Run quotes every element.
func qdeviceSetupArgv(address, network string, force bool) []string {
	argv := []string{pvecmBinary, "qdevice", "setup", address}
	if force {
		argv = append(argv, "--force")
	}
	if network != "" {
		argv = append(argv, "--network", network)
	}
	return argv
}

func qdeviceRemoveArgv() []string {
	return []string{pvecmBinary, "qdevice", "remove"}
}

// parseQDeviceStatus decodes GET /cluster/config/qdevice. PVE answers an
// empty object while no QDevice is configured on the responding node, so
// ok=false means "not configured".
func parseQDeviceStatus(payload json.RawMessage) (st qdeviceStatus, ok bool, err error) {
	if len(payload) == 0 {
		return st, false, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return st, false, fmt.Errorf("decoding QDevice status: %w", err)
	}
	str := func(key string) string {
		switch v := raw[key].(type) {
		case nil:
			return ""
		case string:
			return v
		default:
			return fmt.Sprint(v)
		}
	}
	st.QNetdHost = str("QNetd host")
	if st.QNetdHost == "" {
		return qdeviceStatus{}, false, nil
	}
	st.Host = st.QNetdHost
	if host, _, splitErr := net.SplitHostPort(st.QNetdHost); splitErr == nil {
		st.Host = host
	}
	st.State = str("State")
	st.Algorithm = str("Algorithm")
	st.Model = str("Model")
	st.TieBreak = str("Tie-breaker")
	return st, true, nil
}

// readQDevice asks the API endpoint's node for its QDevice status.
func readQDevice(ctx context.Context, c *pveclient.Client) (qdeviceStatus, bool, error) {
	payload, err := c.DoRequest(ctx, "GET", "/cluster/config/qdevice", nil)
	if err != nil {
		return qdeviceStatus{}, false, err
	}
	return parseQDeviceStatus(payload)
}

func (m *clusterQDeviceModel) apply(st qdeviceStatus) {
	m.QNetdHost = types.StringValue(st.QNetdHost)
	m.State = types.StringValue(st.State)
	m.Algorithm = types.StringValue(st.Algorithm)
	m.Model = types.StringValue(st.Model)
	m.TieBreak = types.StringValue(st.TieBreak)
}

// provision makes sure a QDevice at m.Address is configured: it adopts a
// matching existing one and otherwise runs `pvecm qdevice setup` on m.Node.
func (r *clusterQDeviceResource) provision(ctx context.Context, m *clusterQDeviceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.client == nil {
		diags.AddError("Client Not Configured", "pve_cluster_qdevice: provider client is not configured.")
		return diags
	}
	shell := r.client.Shell()
	if shell == nil {
		diags.AddError(
			"SSH Not Configured",
			"pve_cluster_qdevice runs `pvecm qdevice setup` over SSH, because the Proxmox VE API cannot configure a QDevice. Add an `ssh { }` block to the provider (or set PVE_SSH_USERNAME with PVE_SSH_PASSWORD / PVE_SSH_PRIVATE_KEY / PVE_SSH_AGENT), or run `pvecm qdevice setup` on a node yourself and import the resource.",
		)
		return diags
	}
	node, address := m.Node.ValueString(), m.Address.ValueString()
	if net.ParseIP(address) == nil {
		diags.AddAttributeError(path.Root("address"), "Invalid QNet Address", fmt.Sprintf("%q is not an IP address; pvecm qdevice setup accepts only an IP.", address))
		return diags
	}

	st, ok, err := readQDevice(ctx, r.client)
	if err != nil {
		diags.AddError("Error Reading QDevice", fmt.Sprintf("GET /cluster/config/qdevice: %s", err))
		return diags
	}
	if ok && st.Host != address {
		diags.AddError("QDevice Already Configured", fmt.Sprintf("A QDevice using QNet host %s is already configured. Import it with `terraform import` (ID %s/%s) or remove it with `pvecm qdevice remove` first.", st.Host, node, st.Host))
		return diags
	}
	if !ok {
		argv := qdeviceSetupArgv(address, m.Network.ValueString(), m.Force.ValueBool())
		runCtx, cancel := context.WithTimeout(ctx, r.client.TaskTimeout())
		defer cancel()
		if _, err := shell.Run(runCtx, node, argv, ""); err != nil {
			diags.AddError("Error Setting Up QDevice", fmt.Sprintf("pvecm qdevice setup on node %s failed: %s", node, err))
			return diags
		}
		st, ok, err = readQDevice(ctx, r.client)
		if err != nil {
			diags.AddError("Error Reading QDevice", fmt.Sprintf("GET /cluster/config/qdevice: %s", err))
			return diags
		}
		if !ok {
			diags.AddError("QDevice Not Configured", fmt.Sprintf("pvecm qdevice setup exited successfully on node %s but the API endpoint's node reports no QDevice.", node))
			return diags
		}
	}
	m.ID = types.StringValue(address)
	m.apply(st)
	return diags
}

func (r *clusterQDeviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m clusterQDeviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.provision(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *clusterQDeviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m clusterQDeviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "pve_cluster_qdevice: provider client is not configured.")
		return
	}
	st, ok, err := readQDevice(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading QDevice", fmt.Sprintf("GET /cluster/config/qdevice: %s", err))
		return
	}
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	// A different QNet host surfaces as a replacement of `address`.
	m.Address = types.StringValue(st.Host)
	m.ID = types.StringValue(st.Host)
	m.apply(st)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update only copies the plan: every configurable attribute forces replacement.
func (r *clusterQDeviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m clusterQDeviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *clusterQDeviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m clusterQDeviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "pve_cluster_qdevice: provider client is not configured.")
		return
	}
	shell := r.client.Shell()
	if shell == nil {
		resp.Diagnostics.AddError("SSH Not Configured", "pve_cluster_qdevice runs `pvecm qdevice remove` over SSH. Add an `ssh { }` block to the provider (or set the PVE_SSH_* variables), or remove the QDevice on a node yourself.")
		return
	}
	node := m.Node.ValueString()
	runCtx, cancel := context.WithTimeout(ctx, r.client.TaskTimeout())
	defer cancel()
	if _, err := shell.Run(runCtx, node, qdeviceRemoveArgv(), ""); err != nil && !strings.Contains(err.Error(), "No QDevice configured") {
		resp.Diagnostics.AddError("Error Removing QDevice", fmt.Sprintf("pvecm qdevice remove on node %s failed: %s", node, err))
	}
}

// ImportState takes `<node>/<address>`; Read fills in the rest.
func (r *clusterQDeviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, address, ok := strings.Cut(req.ID, "/")
	if !ok || !nodeNameRe.MatchString(node) || net.ParseIP(address) == nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected <node>/<address> (for example pve01/10.0.0.50), got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("address"), address)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), address)...)
}
