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
	"time"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	Username     types.String `tfsdk:"username"`
	Password     types.String `tfsdk:"password"`
	TOTP         types.String `tfsdk:"totp"`
	Insecure     types.Bool   `tfsdk:"insecure"`
	CACert       types.String `tfsdk:"ca_cert"`
	Timeout      types.Int64  `tfsdk:"request_timeout"`
	SkipValidate types.Bool   `tfsdk:"skip_credentials_validation"`
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
				MarkdownDescription: "API token in the form `user@realm!tokenid=uuid`. Takes precedence over username/password. May also be set via the `PVE_API_TOKEN` environment variable.",
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
			"skip_credentials_validation": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip the reachability and authentication check performed at provider configuration time.",
			},
		},
	}
}

func (p *pveProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config pveProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Guard against unknown values (e.g. api_token = some_resource.output):
	// resolving them as empty would silently authenticate as nobody.
	for name, value := range map[string]types.String{
		"api_url":   config.APIURL,
		"api_token": config.APIToken,
		"username":  config.Username,
		"password":  config.Password,
		"totp":      config.TOTP,
		"ca_cert":   config.CACert,
	} {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Unknown Value During Provider Configuration",
				fmt.Sprintf("The provider cannot configure the client because %s depends on a value known only after apply. Set a static value or use the corresponding PVE_* environment variable.", name),
			)
		}
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
		APIToken: config.APIToken.ValueString(),
		Username: config.Username.ValueString(),
		Password: config.Password.ValueString(),
		TOTP:     config.TOTP.ValueString(),
		Getenv:   p.getenv,
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

	client, err := pveclient.NewClient(pveclient.Config{
		URL:            apiURL,
		Credentials:    creds,
		Insecure:       insecure,
		CACertPEM:      caCert,
		RequestTimeout: timeout,
	})
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
			addConnectionDiagnostic(resp.Diagnostics, apiURL, creds, err)
			return
		}
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func addConnectionDiagnostic(diagnostics diag.Diagnostics, apiURL string, creds pveclient.Credentials, err error) {
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
	return nil
}

func (p *pveProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVersionDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &pveProvider{
			version: version,
			getenv:  os.Getenv,
		}
	}
}
