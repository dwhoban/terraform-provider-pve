// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure pveProvider satisfies the provider interfaces it implements.
var _ provider.Provider = &pveProvider{}

// pveProvider defines the provider implementation.
type pveProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string

	// getenv is injectable for unit tests and defaults to os.Getenv.
	getenv func(string) string
}

// pveProviderModel describes the provider data model.
type pveProviderModel struct {
	APIURL       types.String `tfsdk:"api_url"`
	APIToken     types.String `tfsdk:"api_token"`
	APITokenID   types.String `tfsdk:"api_token_id"`
	APISecret    types.String `tfsdk:"api_secret"`
	Username     types.String `tfsdk:"username"`
	Password     types.String `tfsdk:"password"`
	TOTP         types.String `tfsdk:"totp"`
	Insecure     types.Bool   `tfsdk:"insecure"`
	CACert       types.String `tfsdk:"ca_cert"`
	Timeout      types.Int64  `tfsdk:"request_timeout"`
	SkipValidate types.Bool   `tfsdk:"skip_credentials_validation"`
	TaskTimeout  types.Int64  `tfsdk:"task_timeout"`
	SSH          *sshModel    `tfsdk:"ssh"`
}

func (p *pveProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pve"
	resp.Version = p.version
}

func (p *pveProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Proxmox VE (PVE) provider manages virtual machines, containers, storage, and cluster configuration on Proxmox Virtual Environment.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Proxmox VE API endpoint, `https://host:8006`. May also be set via the `PVE_API_URL` environment variable.",
			},
			"api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "API token in the form `user@realm!tokenid=uuid`. Takes precedence over username/password. May also be set via the `PVE_API_TOKEN` environment variable. Mutually exclusive with `api_token_id`/`api_secret`.",
			},
			"api_token_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API token ID in the form `user@realm!tokenid`, as shown in the Proxmox dashboard, e.g. `root@pam!terraform`. Requires `api_secret`. Mutually exclusive with `api_token`. May also be set via the `PVE_API_TOKEN_ID` environment variable.",
			},
			"api_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "API token secret (the UUID token value shown once at token generation). Requires `api_token_id`. Mutually exclusive with `api_token`. May also be set via the `PVE_API_SECRET` environment variable.",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Username for session-ticket auth, e.g. `root@pam`. Requires `password`. May also be set via the `PVE_USER` environment variable.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for `username`. May also be set via the `PVE_PASSWORD` environment variable.",
			},
			"totp": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "TOTP one-time password for two-factor auth with username/password. May also be set via the `PVE_TOTP` environment variable.",
			},
			"insecure": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip TLS certificate verification. May also be set via the `PVE_INSECURE` environment variable (`1`/`true`).",
			},
			"ca_cert": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PEM-encoded CA bundle used to verify the API endpoint. May also be set via the `PVE_CA_CERT` environment variable.",
			},
			"request_timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Per-request timeout in seconds. Defaults to 30. May also be set via the `PVE_REQUEST_TIMEOUT` environment variable.",
			},
			"task_timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "How long to wait for a single Proxmox VE task (UPID) to finish before failing, in seconds. Applies to every create, update, delete, and action that returns a task. Defaults to 1800 (30 minutes). May also be set via the `PVE_TASK_TIMEOUT` environment variable.",
			},
			"skip_credentials_validation": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip the reachability and authentication check performed at provider configuration time.",
			},
		},
		Blocks: map[string]schema.Block{"ssh": sshBlock()},
	}
}

func (p *pveProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config pveProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Guard against unknown values (e.g. api_token = some_resource.output):
	// resolving them as empty would silently authenticate as nobody. The
	// same applies to the boolean/number settings, whose unknown values
	// coerce to false/0 and would silently override env fallbacks.
	for name, ev := range map[string]string{
		"api_url": pveclient.EnvAPIURL, "api_token": pveclient.EnvAPIToken,
		"api_token_id": pveclient.EnvAPITokenID, "api_secret": pveclient.EnvAPISecret,
		"username": pveclient.EnvUser, "password": pveclient.EnvPassword,
		"totp": pveclient.EnvTOTP, "ca_cert": pveclient.EnvCACert,
	} {
		var value types.String
		switch name {
		case "api_url":
			value = config.APIURL
		case "api_token":
			value = config.APIToken
		case "api_token_id":
			value = config.APITokenID
		case "api_secret":
			value = config.APISecret
		case "username":
			value = config.Username
		case "password":
			value = config.Password
		case "totp":
			value = config.TOTP
		case "ca_cert":
			value = config.CACert
		}
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Unknown Value During Provider Configuration",
				fmt.Sprintf("The provider cannot configure the client because %s depends on a value known only after apply. Set a static value or use the %s environment variable.", name, ev),
			)
		}
	}
	if config.Insecure.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure"),
			"Unknown Value During Provider Configuration",
			fmt.Sprintf("The provider cannot resolve insecure because it depends on a value known only after apply. Set a static value or use %s.", pveclient.EnvInsecure),
		)
	}
	if config.Timeout.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("request_timeout"),
			"Unknown Value During Provider Configuration",
			fmt.Sprintf("The provider cannot resolve request_timeout because it depends on a value known only after apply. Set a static value or use %s.", pveclient.EnvTimeout),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if p.getenv == nil {
		p.getenv = os.Getenv
	}

	// Credentials resolve as a set: static configuration first, then
	// environment variables.
	creds, err := pveclient.ResolveCredentials(pveclient.ResolveOptions{
		APIToken:   config.APIToken.ValueString(),
		APITokenID: config.APITokenID.ValueString(),
		APISecret:  config.APISecret.ValueString(),
		Username:   config.Username.ValueString(),
		Password:   config.Password.ValueString(),
		TOTP:       config.TOTP.ValueString(),
		Getenv:     p.getenv,
	})
	if err != nil {
		if errors.Is(err, pveclient.ErrNoCredentials) {
			resp.Diagnostics.AddError("No Valid Credential Sources Found", err.Error())
		} else {
			resp.Diagnostics.AddError("Failed to Resolve Proxmox VE Credentials", err.Error())
		}
		return
	}

	// Connection settings resolve field-by-field: config > env > default.
	apiURL := config.APIURL.ValueString()
	if apiURL == "" {
		apiURL = p.getenv(pveclient.EnvAPIURL)
	}
	if apiURL == "" {
		resp.Diagnostics.AddError(
			"Missing Proxmox VE API URL",
			fmt.Sprintf("api_url is not set in the provider block and %s is empty. Set it to the API endpoint, e.g. https://pve.example.com:8006.", pveclient.EnvAPIURL),
		)
		return
	}

	insecure := config.Insecure.ValueBool()
	if config.Insecure.IsNull() {
		insecure = envBool(p.getenv, pveclient.EnvInsecure)
	}

	caCert := config.CACert.ValueString()
	if caCert == "" {
		caCert = p.getenv(pveclient.EnvCACert)
	}

	timeout := time.Duration(0)
	if !config.Timeout.IsNull() && config.Timeout.ValueInt64() > 0 {
		timeout = time.Duration(config.Timeout.ValueInt64()) * time.Second
	} else if v := p.getenv(pveclient.EnvTimeout); v != "" {
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil || secs <= 0 {
			resp.Diagnostics.AddError(
				"Invalid Request Timeout",
				fmt.Sprintf("%s=%q is not a positive integer number of seconds.", pveclient.EnvTimeout, v),
			)
			return
		}
		timeout = time.Duration(secs) * time.Second
	}

	taskTimeout := time.Duration(0)
	if !config.TaskTimeout.IsNull() && config.TaskTimeout.ValueInt64() > 0 {
		taskTimeout = time.Duration(config.TaskTimeout.ValueInt64()) * time.Second
	} else if v := p.getenv(pveclient.EnvTaskTimeout); v != "" {
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil || secs <= 0 {
			resp.Diagnostics.AddError(
				"Invalid Task Timeout",
				fmt.Sprintf("%s=%q is not a positive integer number of seconds.", pveclient.EnvTaskTimeout, v),
			)
			return
		}
		taskTimeout = time.Duration(secs) * time.Second
	}

	clientCfg := pveclient.Config{
		URL:            apiURL,
		Credentials:    creds,
		Insecure:       insecure,
		CACertPEM:      caCert,
		RequestTimeout: timeout,
		TaskTimeout:    taskTimeout,
	}

	// Optional SSH (lazy: nothing connects until an SSH-backed operation
	// runs). The resolver uses a separate client without root-ticket
	// minting, because minting itself needs the node address.
	if sshConfigured(config.SSH, p.getenv) {
		runner := buildSSHRunner(config.SSH, p.getenv, creds, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		lookup, err := pveclient.NewClient(clientCfg)
		if err != nil {
			resp.Diagnostics.AddError("Invalid Proxmox VE Provider Configuration", err.Error())
			return
		}
		runner.SetResolver(apiNodeResolver(lookup))
		clientCfg.Shell = runner
		if runner.RootTicketEnabled() {
			clientCfg.RootTickets = runner
		}
	}

	client, err := pveclient.NewClient(clientCfg)
	if err != nil {
		if errors.Is(err, pveclient.ErrNoCredentials) {
			resp.Diagnostics.AddError("No Valid Credential Sources Found", err.Error())
		} else {
			resp.Diagnostics.AddError("Invalid Proxmox VE Provider Configuration", err.Error())
		}
		return
	}

	// Eager validation: fail at plan time with an actionable message rather
	// than mid-apply. Cheap identity check via GET /version.
	if !config.SkipValidate.ValueBool() {
		if _, err := client.GetVersion(ctx); err != nil {
			addConnectionDiagnostic(&resp.Diagnostics, apiURL, creds, err)
			return
		}
	}

	resp.DataSourceData = client
	resp.ResourceData = client
	resp.ActionData = client
	sharedProviderClient.set(client)
}

// sharedProviderClient hands the configured client to provider-defined
// functions: framework v1.19.0 has no per-function configure hook, and each
// plugin process serves exactly one configured provider instance.
var sharedProviderClient = &clientHolder{}

type clientHolder struct {
	mu sync.RWMutex
	c  *pveclient.Client
}

func (h *clientHolder) set(c *pveclient.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.c = c
}

func (h *clientHolder) get() *pveclient.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.c
}

func addConnectionDiagnostic(diagnostics *diag.Diagnostics, apiURL string, creds pveclient.Credentials, err error) {
	if pveclient.IsUnauthorized(err) {
		diagnostics.AddError(
			"Invalid Proxmox VE Credentials",
			fmt.Sprintf("The credentials resolved from %s were rejected by %s: %s", creds.Source, apiURL, err),
		)
		return
	}
	diagnostics.AddError(
		"Unable to Reach Proxmox VE API",
		fmt.Sprintf("Connecting to %s failed: %s\n\nIf the endpoint uses a self-signed certificate, set ca_cert to the PEM bundle or insecure = true.", apiURL, err),
	)
}

func envBool(getenv func(string) string, key string) bool {
	switch strings.ToLower(getenv(key)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func (p *pveProvider) Resources(ctx context.Context) []func() resource.Resource {
	return registryConstructors(KindResource, func(d *TypeDescriptor) func() resource.Resource { return NewGenericResource(d) })
}

func (p *pveProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	base := []func() datasource.DataSource{NewVersionDataSource}
	return append(base, registryConstructors(KindDataSource, func(d *TypeDescriptor) func() datasource.DataSource { return NewGenericDataSource(d) })...)
}

func (p *pveProvider) Actions(ctx context.Context) []func() action.Action {
	base := []func() action.Action{NewCephInstallAction}
	return append(base, registryConstructors(KindAction, func(d *TypeDescriptor) func() action.Action { return NewGenericAction(d) })...)
}

func (p *pveProvider) Functions(ctx context.Context) []func() function.Function {
	return registryConstructors(KindFunction, func(d *TypeDescriptor) func() function.Function { return NewGenericFunction(d) })
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &pveProvider{
			version: version,
			getenv:  os.Getenv,
		}
	}
}
