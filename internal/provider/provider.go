// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/credentials"
	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure PveProvider satisfies various provider interfaces.
var _ provider.Provider = &PveProvider{}
var _ provider.ProviderWithFunctions = &PveProvider{}
var _ provider.ProviderWithEphemeralResources = &PveProvider{}
var _ provider.ProviderWithActions = &PveProvider{}

// PveProvider defines the provider implementation.
type PveProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// PveProviderModel describes the provider data model. Field names
// must match the tfsdk tags used in Schema(). The framework decodes config
// into this struct on every Configure call.
type PveProviderModel struct {
	Endpoint                  types.String `tfsdk:"endpoint"`
	APIToken                  types.String `tfsdk:"api_token"`
	Username                  types.String `tfsdk:"username"`
	Password                  types.String `tfsdk:"password"`
	Insecure                  types.String `tfsdk:"insecure"`
	RootCA                    types.String `tfsdk:"root_ca"`
	OTP                       types.String `tfsdk:"otp"`
	SkipCredentialsValidation types.Bool   `tfsdk:"skip_credentials_validation"`
}

func (p *PveProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pve"
	resp.Version = p.version
}

func (p *PveProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Proxmox VE endpoint URL, e.g. `https://pve.example.com:8006/`. May also be set via the `PROXMOX_VE_ENDPOINT` environment variable.",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Proxmox VE API token in the form `USER@REALM!TOKENID=UUID`. May also be set via the `PROXMOX_VE_API_TOKEN` environment variable. Mutually exclusive in effect with `username`/`password`; if both are supplied the token is used.",
				Optional:            true,
				Sensitive:           true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Proxmox VE username in `user@realm` form (e.g. `root@pam`). May also be set via the `PROXMOX_VE_USERNAME` environment variable.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Proxmox VE password for `username`. May also be set via the `PROXMOX_VE_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"insecure": schema.StringAttribute{
				MarkdownDescription: "Skip TLS verification of the Proxmox endpoint. Accepts `true` or `1`. May also be set via the `PROXMOX_VE_INSECURE` environment variable. Prefer `root_ca` for production clusters.",
				Optional:            true,
			},
			"root_ca": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded CA bundle used to validate the Proxmox endpoint certificate. May also be set via the `PROXMOX_VE_ROOT_CA` environment variable.",
				Optional:            true,
			},
			"otp": schema.StringAttribute{
				MarkdownDescription: "Optional one-time password used together with `username`/`password` when the target account has TOTP 2FA enabled. May also be set via the `PROXMOX_VE_OTP` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"skip_credentials_validation": schema.BoolAttribute{
				MarkdownDescription: "Skip the `GET /access/whoami` identity check performed during provider configuration. Useful for ephemeral environments where the credentials are known to be valid. Defaults to `false`.",
				Optional:            true,
			},
		},
	}
}

func (p *PveProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data PveProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reject unknown values for the auth attributes so the user gets a clear
	// error during planning instead of a confusing message later. Endpoint
	// and root_ca can legitimately be planned from another resource via
	// for_each and are not gated.
	for _, f := range []struct {
		name  string
		value any
	}{
		{"api_token", data.APIToken},
		{"username", data.Username},
		{"password", data.Password},
		{"otp", data.OTP},
	} {
		if v, ok := f.value.(interface{ IsUnknown() bool }); ok && v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(f.name),
				"Provider configuration value is unknown",
				fmt.Sprintf("The %q attribute is unknown at configuration time. Set it to a literal value or supply the matching PROXMOX_VE_* environment variable.", f.name),
			)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	static := credentials.Credentials{
		Token:    data.APIToken.ValueString(),
		Username: data.Username.ValueString(),
		Password: data.Password.ValueString(),
		Endpoint: data.Endpoint.ValueString(),
		Insecure: data.Insecure.ValueString(),
		RootCA:   data.RootCA.ValueString(),
		OTP:      data.OTP.ValueString(),
	}

	creds, err := credentials.NewDefaultChain(static, credentials.Options{
		GetEnv: os.Getenv,
	}).Retrieve(ctx)
	if err != nil {
		if errors.Is(err, credentials.ErrNoCredentials) {
			var ce *credentials.ChainError
			if errors.As(err, &ce) {
				resp.Diagnostics.AddError(
					"No Proxmox VE credentials found",
					ce.Error()+"\n\nProvide credentials via the provider block, the PROXMOX_VE_* environment variables, or a credentials file at ~/.proxmox/credentials. See https://github.com/hashicorp/terraform-provider-scaffolding-framework/blob/main/docs/index.md for details.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Unable to resolve Proxmox VE credentials",
			fmt.Sprintf("Credential resolution failed: %s", err.Error()),
		)
		return
	}

	client, err := pveclient.NewClient(pveclient.Credentials{
		Token:    creds.Token,
		Username: creds.Username,
		Password: creds.Password,
		Endpoint: creds.Endpoint,
		Insecure: creds.Insecure,
		RootCA:   creds.RootCA,
		OTP:      creds.OTP,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to build Proxmox VE client",
			fmt.Sprintf("Build client from credentials source %q failed: %s", creds.Source, err.Error()),
		)
		return
	}

	if !data.SkipCredentialsValidation.ValueBool() {
		_, err := client.Whoami(ctx)
		if err != nil {
			failingField := "api_token"
			envVar := "PROXMOX_VE_API_TOKEN"
			if creds.Token == "" {
				failingField = "username/password"
				envVar = "PROXMOX_VE_USERNAME / PROXMOX_VE_PASSWORD"
			}
			resp.Diagnostics.AddError(
				"Proxmox VE credential validation failed",
				fmt.Sprintf("GET /access/whoami against %s failed using credentials from %q (auth_kind=%s). Verify the %s value and the matching %s environment variable, or set skip_credentials_validation=true to defer this check. Underlying error: %s",
					client.Endpoint(), creds.Source, client.AuthKind(), failingField, envVar, err.Error()),
			)
			return
		}
	}

	// Protocol 6 surfaces five data slots. Mirror the client across all of
	// them so future resources, data sources, actions, ephemeral resources,
	// and functions can receive the same configured client.
	resp.DataSourceData = client
	resp.ResourceData = client
	resp.ActionData = client
	resp.EphemeralResourceData = client

	// Functions have no configure hook in framework v1.19; publish the
	// client through the package-level slot so pve_next_id can reach it.
	pveNextIdClientSlot.Store(client)
}

func (p *PveProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPveNodeResource,
		NewPveNodeNetworkLinuxBridgeResource,
		NewPveNodeNetworkLinuxBondResource,
		NewPveNodeNetworkVlanResource,
		NewPveNodeDiskZFSResource,
		NewPveUserResource,
		NewPveUserTokenResource,
		NewPveGroupResource,
		NewPveRoleResource,
		NewPveAclResource,
		NewPveRealmLdapResource,
		NewPveRealmAdResource,
		NewPveRealmOpenidResource,
		NewPveRealmSyncJobResource,
		NewPveClusterNodeResource,
		NewPveClusterOptionsResource,
		NewPveHaGroupResource,
		NewPveHaResourceResource,
		NewPveHaRuleResource,
		NewPveClusterFirewallOptionsResource,
		NewPveNodeFirewallOptionsResource,
		NewPveGuestFirewallOptionsResource,
		NewPveSdnFirewallOptionsResource,
		NewPveFirewallAliasResource,
		NewPveFirewallIpsetResource,
		NewPveFirewallSecurityGroupResource,
		NewPveClusterFirewallRulesResource,
		NewPveNodeFirewallRulesResource,
		NewPveGuestFirewallRulesResource,
		NewPveSecurityGroupFirewallRulesResource,
		NewPveVnetFirewallRulesResource,
		NewPveBackupJobResource,
		NewPveReplicationResource,
		NewPveMetricsServerResource,
		NewPveNotificationEndpointSendmailResource,
		NewPveNotificationEndpointGotyResource,
		NewPveNotificationEndpointSMTPResource,
		NewPveNotificationEndpointWebhookResource,
		NewPveNotificationMatcherResource,
		NewPvePoolResource,
		NewPveStorageNfsResource,
		NewPveStorageCifsResource,
		NewPveStorageIscsiResource,
		NewPveStorageIscsidirectResource,
		NewPveStorageLvmResource,
		NewPveStorageLvmthinResource,
		NewPveStorageZfspoolResource,
		NewPveStorageDirectoryResource,
		NewPveStoragePbsResource,
		NewPveStorageCephfsResource,
		NewPveStorageRbdResource,
		NewPveFileResource,
		NewPveDownloadFileResource,
		NewPveNodeHostsResource,
		NewPveNodeDiskLvmthinResource,
		NewPveNodeDiskDirectoryResource,
		NewPveCephPoolResource,
		NewPveCephOSDResource,
		NewPveCephMonResource,
		NewPveAcmeAccountResource,
		NewPveAcmeDnsPluginResource,
		NewPveHardwareMappingPciResource,
		NewPveHardwareMappingUsbResource,
		NewPveMappingDirResource,
		NewPveCustomCPUModelResource,
		NewPveNodeCertificateResource,
		NewPveAcmeCertificateResource,
		NewPveAptStandardRepositoryResource,
		NewPveVmResource,
		NewPveVmSnapshotResource,
		NewPveContainerResource,
		NewPveContainerSnapshotResource,
		NewPveSdnZoneSimpleResource,
		NewPveSdnZoneVlanResource,
		NewPveSdnZoneQinqResource,
		NewPveSdnZoneVxlanResource,
		NewPveSdnZoneEvpnResource,
		NewPveSdnVnetResource,
		NewPveSdnSubnetResource,
		NewPveSdnControllerResource,
		NewPveSdnDnsResource,
		NewPveSdnIpamResource,
		NewPveSdnPrefixListResource,
		NewPveSdnRouteMapResource,
		NewPveSdnFabricOspfResource,
		NewPveSdnFabricOpenfabricResource,
	}
}

func (p *PveProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}

func (p *PveProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPveNodesDataSource,
		NewPveNodeStatusDataSource,
		NewPveNodeDisksDataSource,
		NewPveNodeNetworkInterfacesDataSource,
		NewPveUserDataSource,
		NewPveUserTokenDataSource,
		NewPveGroupDataSource,
		NewPveRoleDataSource,
		NewPveAclDataSource,
		NewPvePermissionsDataSource,
		NewPveRealmLdapDataSource,
		NewPveRealmAdDataSource,
		NewPveRealmOpenidDataSource,
		NewPveRealmsDataSource,
		NewPveRealmSyncJobDataSource,
		NewPveClusterResourcesDataSource,
		NewPveClusterStatusDataSource,
		NewPveTasksDataSource,
		NewPveVersionDataSource,
		NewPveClusterNodeDataSource,
		NewPveClusterOptionsDataSource,
		NewPveHaStatusDataSource,
		NewPveHaGroupDataSource,
		NewPveHaResourceDataSource,
		NewPveHaRuleDataSource,
		NewPveClusterFirewallOptionsDataSource,
		NewPveNodeFirewallOptionsDataSource,
		NewPveGuestFirewallOptionsDataSource,
		NewPveSdnFirewallOptionsDataSource,
		NewPveFirewallAliasDataSource,
		NewPveFirewallIpsetDataSource,
		NewPveFirewallSecurityGroupDataSource,
		NewPveBackupJobDataSource,
		NewPveBackupJobsDataSource,
		NewPveReplicationDataSource,
		NewPveNodeReplicationsDataSource,
		NewPveMetricsServerDataSource,
		NewPveNotificationEndpointSendmailDataSource,
		NewPveNotificationEndpointGotyDataSource,
		NewPveNotificationEndpointSMTPDataSource,
		NewPveNotificationEndpointWebhookDataSource,
		NewPveNotificationMatcherDataSource,
		NewPveNotificationTargetsDataSource,
		NewPvePoolDataSource,
		NewPveStorageNfsDataSource,
		NewPveStorageCifsDataSource,
		NewPveStorageIscsiDataSource,
		NewPveStorageIscsidirectDataSource,
		NewPveStorageLvmDataSource,
		NewPveStorageLvmthinDataSource,
		NewPveStorageZfspoolDataSource,
		NewPveStorageDirectoryDataSource,
		NewPveStoragePbsDataSource,
		NewPveStorageCephfsDataSource,
		NewPveStorageRbdDataSource,
		NewPveNodeStoragesDataSource,
		NewPveStorageFilesDataSource,
		NewPveFileDataSource,
		NewPveDownloadFileDataSource,
		NewPveNodeDataSource,
		NewPveNodeHostsDataSource,
		NewPveNodeDiskLvmthinDataSource,
		NewPveNodeDiskDirectoryDataSource,
		NewPveNodeServicesDataSource,
		NewPveNodeAptRepositoriesDataSource,
		NewPveAptStandardRepositoryDataSource,
		NewPveNodeTasksDataSource,
		NewPveNodePciDevicesDataSource,
		NewPveNodeUsbDevicesDataSource,
		NewPveNodeCapabilitiesDataSource,
		NewPveCephStatusDataSource,
		NewPveCephPoolDataSource,
		NewPveCephOSDDataSource,
		NewPveCephMonDataSource,
		NewPveAcmeAccountDataSource,
		NewPveAcmePluginsDataSource,
		NewPveAcmeDnsPluginDataSource,
		NewPveHardwareMappingPciDataSource,
		NewPveHardwareMappingUsbDataSource,
		NewPveMappingDirDataSource,
		NewPveCustomCPUModelDataSource,
		NewPveNodeCertificateDataSource,
		NewPveAcmeCertificateDataSource,
		NewPveVmsDataSource,
		NewPveVmDataSource,
		NewPveVmSnapshotDataSource,
		NewPveContainersDataSource,
		NewPveContainerDataSource,
		NewPveContainerSnapshotDataSource,
		NewPveSdnZoneSimpleDataSource,
		NewPveSdnZoneVlanDataSource,
		NewPveSdnZoneQinqDataSource,
		NewPveSdnZoneVxlanDataSource,
		NewPveSdnZoneEvpnDataSource,
		NewPveSdnVnetDataSource,
		NewPveSdnSubnetDataSource,
		NewPveSdnControllerDataSource,
		NewPveSdnDnsDataSource,
		NewPveSdnIpamDataSource,
		NewPveSdnPrefixListDataSource,
		NewPveSdnRouteMapDataSource,
		NewPveSdnFabricOspfDataSource,
		NewPveSdnFabricOpenfabricDataSource,
		NewPveAppliancesDataSource,
		NewPveVmAgentInfoDataSource,
		NewPveNodeSubscriptionDataSource,
	}
}

func (p *PveProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{
		NewPveNextIdFunction,
	}
}

func (p *PveProvider) Actions(ctx context.Context) []func() action.Action {
	return []func() action.Action{
		NewPveRealmSyncAction,
		NewPveHaArmAction,
		NewPveNotificationTestAction,
		NewPveStoragePruneBackupsAction,
		NewPveNodeRebootAction,
		NewPveNodeShutdownAction,
		NewPveNodeServiceAction,
		NewPveNodeAptUpdateAction,
		NewPveNodeWakeonlanAction,
		NewPveVmRebootAction,
		NewPveVmSuspendAction,
		NewPveVmResumeAction,
		NewPveVmResetAction,
		NewPveVmMigrateAction,
		NewPveVmSnapshotRollbackAction,
		NewPveContainerRebootAction,
		NewPveContainerSuspendAction,
		NewPveContainerResumeAction,
		NewPveContainerMigrateAction,
		NewPveContainerSnapshotRollbackAction,
		NewPveSdnApplyAction,
		NewPveSdnRollbackAction,
		NewPveUserTfaUnlockAction,
		NewPveHaResourceMigrateAction,
		NewPveHaResourceRelocateAction,
		NewPveReplicationScheduleNowAction,
		NewPveBackupRunAction,
		NewPveSubscriptionRefreshAction,
		NewPveNodeDiskInitgptAction,
		NewPveTaskCancelAction,
		NewPveAplinfoUpdateAction,
		NewPveStorageOciPullAction,
		NewPveNodeExecuteAction,
		NewPveNodeStartAllAction,
		NewPveNodeStopAllAction,
		NewPveNodeSuspendAllAction,
		NewPveNodeMigrateAllAction,
		NewPveGuestBulkStartAction,
		NewPveGuestBulkShutdownAction,
		NewPveGuestBulkSuspendAction,
		NewPveGuestBulkMigrateAction,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PveProvider{
			version: version,
		}
	}
}
