// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pvecephBinary is the absolute path granted in sudoers.
const pvecephBinary = "/usr/sbin/pveceph"

var (
	cephNodeRe    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)
	cephVersionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// cephInstallAction installs the Ceph packages on a node with
// `pveceph install`. The PVE API has no endpoint for this, so it is the one
// action that needs the provider's ssh block. It is idempotent: when the
// node's Ceph API already answers, nothing is run.
type cephInstallAction struct {
	client *pveclient.Client
}

var (
	_ action.Action              = (*cephInstallAction)(nil)
	_ action.ActionWithConfigure = (*cephInstallAction)(nil)
)

// NewCephInstallAction is the action constructor.
func NewCephInstallAction() action.Action { return &cephInstallAction{} }

type cephInstallModel struct {
	Node              types.String `tfsdk:"node"`
	Repository        types.String `tfsdk:"repository"`
	Version           types.String `tfsdk:"version"`
	AllowExperimental types.Bool   `tfsdk:"allow_experimental"`
	Force             types.Bool   `tfsdk:"force"`
}

func (a *cephInstallAction) Metadata(_ context.Context, _ action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = "pve_ceph_install"
}

func (a *cephInstallAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Install the Ceph packages on a node (`pveceph install`). **Requires the provider's `ssh` block**: the Proxmox VE API has no install endpoint, so this action runs `/usr/sbin/pveceph install` on the node over SSH (as `root`, or through `sudo -n` — see the provider guide for the exact sudoers rule). It is idempotent: if Ceph is already installed it does nothing unless `force` is set. Run it before `pve_ceph_init`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Node to install Ceph on.",
				Validators:          []validator.String{stringvalidator.RegexMatches(cephNodeRe, "must be a valid node name")},
			},
			"repository": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Ceph APT repository: `enterprise` (needs a subscription), `no-subscription`, or `test`. Defaults to `no-subscription`.",
				Validators:          []validator.String{stringvalidator.OneOf("enterprise", "no-subscription", "test")},
			},
			"version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Ceph release code name to install (for example `squid`). Defaults to the release `pveceph` recommends.",
				Validators:          []validator.String{stringvalidator.RegexMatches(cephVersionRe, "must be a lowercase release name such as squid")},
			},
			"allow_experimental": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow experimental Ceph versions (`--allow-experimental`).",
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Run `pveceph install` even if Ceph already responds on the node (for example to upgrade packages).",
			},
		},
	}
}

func (a *cephInstallAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", fmt.Sprintf("Expected *pveclient.Client for pve_ceph_install, got: %T.", req.ProviderData))
		return
	}
	a.client = c
}

// cephInstalled probes the node's Ceph API: PVE answers "binary not
// installed" until the packages exist.
func cephInstalled(ctx context.Context, c *pveclient.Client, node string) (bool, error) {
	_, err := c.DoRequest(ctx, "GET", "/nodes/"+node+"/ceph/osd", nil)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "binary not installed") {
		return false, nil
	}
	// An authorization failure says nothing about the packages.
	if pveclient.IsUnauthorized(err) {
		return false, err
	}
	// Any other API answer (for example "not initialized") means the
	// binaries are present.
	var apiErr *pveclient.APIError
	if errors.As(err, &apiErr) {
		return true, nil
	}
	return false, err
}

// cephInstallArgv builds the exact remote command; values are validated by
// the schema, and Run quotes every element.
func cephInstallArgv(m cephInstallModel) []string {
	repo := "no-subscription"
	if !m.Repository.IsNull() && m.Repository.ValueString() != "" {
		repo = m.Repository.ValueString()
	}
	argv := []string{pvecephBinary, "install", "--repository", repo}
	if !m.Version.IsNull() && m.Version.ValueString() != "" {
		argv = append(argv, "--version", m.Version.ValueString())
	}
	if m.AllowExperimental.ValueBool() {
		argv = append(argv, "--allow-experimental")
	}
	return argv
}

func (a *cephInstallAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var m cephInstallModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Node.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Unknown Value", "node must be known when the action runs.")
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "pve_ceph_install: provider client is not configured.")
		return
	}
	node := m.Node.ValueString()
	progress := func(msg string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: msg})
		}
	}

	if !m.Force.ValueBool() {
		installed, err := cephInstalled(ctx, a.client, node)
		if err != nil {
			resp.Diagnostics.AddError("Error Checking Ceph Installation", fmt.Sprintf("probing Ceph on node %s: %s", node, err))
			return
		}
		if installed {
			progress(fmt.Sprintf("Ceph is already installed on %s; nothing to do (set force = true to run pveceph install anyway)", node))
			return
		}
	}

	shell := a.client.Shell()
	if shell == nil {
		resp.Diagnostics.AddError(
			"SSH Not Configured",
			"pve_ceph_install runs `pveceph install` on the node over SSH, because the Proxmox VE API cannot install packages. Add an `ssh { }` block to the provider (or set PVE_SSH_USERNAME with PVE_SSH_PASSWORD / PVE_SSH_PRIVATE_KEY / PVE_SSH_AGENT), or run `pveceph install` on the node yourself.",
		)
		return
	}

	argv := cephInstallArgv(m)
	progress(fmt.Sprintf("Running on %s: %s (this downloads packages and can take several minutes)", node, strings.Join(argv, " ")))
	runCtx, cancel := context.WithTimeout(ctx, a.client.TaskTimeout())
	defer cancel()
	// "y" answers pveceph's confirmation prompt when it asks one.
	out, err := shell.Run(runCtx, node, argv, "y\n")
	if err != nil {
		resp.Diagnostics.AddError("Error Installing Ceph", fmt.Sprintf("pveceph install on node %s failed: %s", node, err))
		return
	}
	progress(fmt.Sprintf("pveceph install finished on %s: %s", node, truncate(lastLine(out.Stdout), 200)))

	installed, err := cephInstalled(ctx, a.client, node)
	if err != nil || !installed {
		resp.Diagnostics.AddError("Ceph Not Installed", fmt.Sprintf("pveceph install exited successfully on node %s but the node's Ceph API still reports the binaries as missing (probe error: %v).", node, err))
		return
	}
	progress(fmt.Sprintf("Ceph is installed on %s", node))
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
