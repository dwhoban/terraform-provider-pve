// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/dwhoban/terraform-provider-pve/internal/pvessh"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Environment variables for the optional ssh block.
const (
	envSSHUsername        = "PVE_SSH_USERNAME"
	envSSHPassword        = "PVE_SSH_PASSWORD"
	envSSHPrivateKey      = "PVE_SSH_PRIVATE_KEY"
	envSSHAgent           = "PVE_SSH_AGENT"
	envSSHAgentSock       = "PVE_SSH_AUTH_SOCK"
	envSSHKnownHosts      = "PVE_SSH_KNOWN_HOSTS"
	envSSHInsecureHostKey = "PVE_SSH_INSECURE_IGNORE_HOST_KEY"
	envSSHRootTicket      = "PVE_SSH_ROOT_TICKET"
)

// sshModel is the optional `ssh { }` provider block. SSH is used only for
// operations the PVE API cannot perform (see the provider docs, "When is SSH
// required?").
type sshModel struct {
	Username              types.String   `tfsdk:"username"`
	Password              types.String   `tfsdk:"password"`
	PrivateKey            types.String   `tfsdk:"private_key"`
	Agent                 types.Bool     `tfsdk:"agent"`
	AgentSocket           types.String   `tfsdk:"agent_socket"`
	KnownHostsFile        types.String   `tfsdk:"known_hosts_file"`
	InsecureIgnoreHostKey types.Bool     `tfsdk:"insecure_ignore_host_key"`
	RootTicket            types.Bool     `tfsdk:"root_ticket"`
	Nodes                 []sshNodeModel `tfsdk:"node"`
}

type sshNodeModel struct {
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
	Port    types.Int64  `tfsdk:"port"`
}

func sshBlock() schema.Block {
	return schema.SingleNestedBlock{
		MarkdownDescription: "Optional SSH access to the nodes. The provider is API-first; SSH is used only for operations the Proxmox VE API cannot perform — currently installing Ceph (`pve_ceph_install`) and, with `root_ticket`, running `root@pam`-only API calls with an API token. If you use neither, omit this block and no SSH connection is ever made. See \"When is SSH required?\" in the provider guide.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SSH user. Defaults to the PAM user of the API credentials (`root@pam` → `root`); required with a non-PAM API token. Non-root users run commands through `sudo -n`, so they need a **strict** sudoers rule. May also be set via `PVE_SSH_USERNAME`.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SSH password. May also be set via `PVE_SSH_PASSWORD`.",
			},
			"private_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Unencrypted PEM/OpenSSH private key. May also be set via `PVE_SSH_PRIVATE_KEY`.",
			},
			"agent": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Authenticate through the SSH agent. May also be set via `PVE_SSH_AGENT`.",
			},
			"agent_socket": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path of the SSH agent socket. Defaults to `SSH_AUTH_SOCK`. May also be set via `PVE_SSH_AUTH_SOCK`.",
			},
			"known_hosts_file": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "known_hosts file used to verify node host keys. Defaults to `~/.ssh/known_hosts`. May also be set via `PVE_SSH_KNOWN_HOSTS`.",
			},
			"insecure_ignore_host_key": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip host key verification. **Not recommended**: it allows machine-in-the-middle attacks. May also be set via `PVE_SSH_INSECURE_IGNORE_HOST_KEY`.",
			},
			"root_ticket": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Run API calls under `/nodes/{node}/` with a `root@pam` ticket minted on that node over SSH, so an API token can perform operations Proxmox VE restricts to `root@pam`. Requires `username = \"root\"`. While enabled, those calls are logged and authorized as `root@pam`, not as the token; calls outside `/nodes/{node}/` (such as `/cluster`) keep using the configured credentials. May also be set via `PVE_SSH_ROOT_TICKET`.",
			},
		},
		Blocks: map[string]schema.Block{
			"node": schema.ListNestedBlock{
				MarkdownDescription: "Per-node SSH address override. Without it the provider uses the node's address from the API (`/nodes/{node}/network`), then the node name as a DNS name.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Node name."},
						"address": schema.StringAttribute{Required: true, MarkdownDescription: "IP address or FQDN used for SSH."},
						"port":    schema.Int64Attribute{Optional: true, MarkdownDescription: "SSH port. Defaults to 22."},
					},
				},
			},
		},
	}
}

// sshConfigured reports whether any SSH setting is present in configuration
// or environment; absent that, no runner is built and SSH-dependent
// operations fail with an actionable message.
func sshConfigured(m *sshModel, getenv func(string) string) bool {
	if m != nil {
		if !m.Username.IsNull() || !m.Password.IsNull() || !m.PrivateKey.IsNull() || !m.Agent.IsNull() ||
			!m.RootTicket.IsNull() || len(m.Nodes) > 0 {
			return true
		}
	}
	for _, k := range []string{envSSHUsername, envSSHPassword, envSSHPrivateKey, envSSHAgent, envSSHRootTicket} {
		if getenv(k) != "" {
			return true
		}
	}
	return false
}

// pamUser extracts the system user of a PAM API identity ("root@pam" or
// "root@pam!tokenid=secret"); ok is false for any other realm.
func pamUser(creds pveclient.Credentials) (string, bool) {
	id := creds.Username
	if creds.UsesToken() {
		id = creds.APIToken
	}
	id, _, _ = strings.Cut(id, "!")
	id, _, _ = strings.Cut(id, "=")
	user, realm, ok := strings.Cut(id, "@")
	if !ok || realm != "pam" || user == "" {
		return "", false
	}
	return user, true
}

// buildSSHRunner resolves the ssh block (config > env) into a Runner.
func buildSSHRunner(m *sshModel, getenv func(string) string, creds pveclient.Credentials, diags *diag.Diagnostics) *pvessh.Runner {
	if m == nil {
		m = &sshModel{}
	}
	for name, v := range map[string]interface{ IsUnknown() bool }{
		"username": m.Username, "password": m.Password, "private_key": m.PrivateKey, "agent": m.Agent,
		"agent_socket": m.AgentSocket, "known_hosts_file": m.KnownHostsFile,
		"insecure_ignore_host_key": m.InsecureIgnoreHostKey, "root_ticket": m.RootTicket,
	} {
		if v.IsUnknown() {
			diags.AddAttributeError(path.Root("ssh").AtName(name), "Unknown Value During Provider Configuration",
				fmt.Sprintf("ssh.%s depends on a value known only after apply. Set a static value or the matching PVE_SSH_* environment variable.", name))
		}
	}
	if diags.HasError() {
		return nil
	}

	str := func(v types.String, env string) string {
		if !v.IsNull() && v.ValueString() != "" {
			return v.ValueString()
		}
		return getenv(env)
	}
	flag := func(v types.Bool, env string) bool {
		if !v.IsNull() {
			return v.ValueBool()
		}
		return envBool(getenv, env)
	}

	cfg := pvessh.Config{
		Username:              str(m.Username, envSSHUsername),
		Password:              str(m.Password, envSSHPassword),
		PrivateKey:            str(m.PrivateKey, envSSHPrivateKey),
		Agent:                 flag(m.Agent, envSSHAgent),
		AgentSock:             str(m.AgentSocket, envSSHAgentSock),
		KnownHostsFile:        str(m.KnownHostsFile, envSSHKnownHosts),
		InsecureIgnoreHostKey: flag(m.InsecureIgnoreHostKey, envSSHInsecureHostKey),
		RootTicket:            flag(m.RootTicket, envSSHRootTicket),
	}
	if cfg.InsecureIgnoreHostKey {
		warning := "SSH host key verification is disabled (ssh.insecure_ignore_host_key): a machine-in-the-middle can intercept SSH commands"
		if cfg.RootTicket {
			warning += " and steal the minted root@pam ticket"
		}
		diags.AddWarning("SSH Host Key Verification Disabled", warning+".")
	}
	if cfg.Username == "" {
		if u, ok := pamUser(creds); ok {
			cfg.Username = u
		}
	}
	if cfg.Password == "" && cfg.PrivateKey == "" && !cfg.Agent && !creds.UsesToken() {
		// Username/password API login of a PAM user: the same user's system
		// password authenticates SSH.
		if u, ok := pamUser(creds); ok && u == cfg.Username {
			cfg.Password = creds.Password
		}
	}
	for i, n := range m.Nodes {
		if n.Name.IsUnknown() || n.Address.IsUnknown() || n.Port.IsUnknown() {
			diags.AddAttributeError(path.Root("ssh").AtName("node").AtListIndex(i), "Unknown Value During Provider Configuration",
				"ssh.node values must be known at plan time.")
			return nil
		}
		cfg.Nodes = append(cfg.Nodes, pvessh.Node{Name: n.Name.ValueString(), Address: n.Address.ValueString(), Port: int(n.Port.ValueInt64())})
	}
	runner, err := pvessh.New(cfg)
	if err != nil {
		diags.AddAttributeError(path.Root("ssh"), "Invalid SSH Configuration", err.Error())
		return nil
	}
	return runner
}

// apiNodeResolver finds a node's SSH address from its network interfaces:
// first the interface with an address and a gateway, then any interface
// with an address. It uses a client without root-ticket minting, since the
// lookup itself is what minting depends on.
func apiNodeResolver(c *pveclient.Client) pvessh.Resolver {
	return func(ctx context.Context, node string) (string, error) {
		raw, err := c.DoRequest(ctx, "GET", "/nodes/"+node+"/network", nil)
		if err != nil {
			return "", err
		}
		var ifaces []struct {
			Address  string `json:"address"`
			Address6 string `json:"address6"`
			Gateway  string `json:"gateway"`
			Gateway6 string `json:"gateway6"`
		}
		if err := json.Unmarshal(raw, &ifaces); err != nil {
			return "", err
		}
		for _, i := range ifaces {
			if i.Address != "" && i.Gateway != "" {
				return i.Address, nil
			}
			if i.Address6 != "" && i.Gateway6 != "" {
				return i.Address6, nil
			}
		}
		for _, i := range ifaces {
			if ip := net.ParseIP(i.Address); ip != nil {
				return i.Address, nil
			}
		}
		return "", fmt.Errorf("node %s reports no interface address", node)
	}
}
