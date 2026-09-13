// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveClusterOptionsResource{}
	_ resource.ResourceWithConfigure   = &pveClusterOptionsResource{}
	_ resource.ResourceWithImportState = &pveClusterOptionsResource{}
)

// pveClusterOptionsID is the static identifier of the datacenter options
// singleton.
const pveClusterOptionsID = "cluster"

// NewPveClusterOptionsResource returns the resource implementation.
func NewPveClusterOptionsResource() resource.Resource {
	return &pveClusterOptionsResource{}
}

// pveClusterOptionsResource manages the datacenter-wide cluster options
// singleton via GET/PUT /cluster/options.
type pveClusterOptionsResource struct {
	client *pveclient.Client
}

// pveClusterOptionsOptionSet carries the option fields shared by the
// resource and data source models; both embed it with its tfsdk tags.
type pveClusterOptionsOptionSet struct {
	BWLimit           *pveClusterOptionsBWLimitModel       `tfsdk:"bwlimit"`
	ConsentText       types.String                         `tfsdk:"consent_text"`
	Console           types.String                         `tfsdk:"console"`
	CRS               *pveClusterOptionsCRSModel           `tfsdk:"crs"`
	Description       types.String                         `tfsdk:"description"`
	EmailFrom         types.String                         `tfsdk:"email_from"`
	Fencing           types.String                         `tfsdk:"fencing"`
	HA                *pveClusterOptionsHAModel            `tfsdk:"ha"`
	HTTPProxy         types.String                         `tfsdk:"http_proxy"`
	Keyboard          types.String                         `tfsdk:"keyboard"`
	Language          types.String                         `tfsdk:"language"`
	Location          *pveClusterOptionsLocationModel      `tfsdk:"location"`
	MACPrefix         types.String                         `tfsdk:"mac_prefix"`
	MaxWorkers        types.Int64                          `tfsdk:"max_workers"`
	Migration         *pveClusterOptionsMigrationModel     `tfsdk:"migration"`
	MigrationUnsecure types.Bool                           `tfsdk:"migration_unsecure"`
	NextID            *pveClusterOptionsNextIDModel        `tfsdk:"next_id"`
	Notify            *pveClusterOptionsNotifyModel        `tfsdk:"notify"`
	RegisteredTags    types.String                         `tfsdk:"registered_tags"`
	Replication       *pveClusterOptionsReplicationModel   `tfsdk:"replication"`
	TagStyle          *pveClusterOptionsTagStyleModel      `tfsdk:"tag_style"`
	U2F               *pveClusterOptionsU2FModel           `tfsdk:"u2f"`
	UserTagAccess     *pveClusterOptionsUserTagAccessModel `tfsdk:"user_tag_access"`
	WebAuthn          *pveClusterOptionsWebAuthnModel      `tfsdk:"webauthn"`
}

// pveClusterOptionsResourceModel is the Terraform-facing shape of the
// resource.
type pveClusterOptionsResourceModel struct {
	pveClusterOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// pveClusterOptionsBWLimitModel mirrors the bwlimit property string
// (KiB/s limits).
type pveClusterOptionsBWLimitModel struct {
	Clone     types.Float64 `tfsdk:"clone"`
	Default   types.Float64 `tfsdk:"default"`
	Migration types.Float64 `tfsdk:"migration"`
	Move      types.Float64 `tfsdk:"move"`
	Restore   types.Float64 `tfsdk:"restore"`
}

// pveClusterOptionsCRSModel mirrors the crs (cluster resource scheduling)
// property string.
type pveClusterOptionsCRSModel struct {
	Ha                          types.String  `tfsdk:"ha"`
	HaAutoRebalance             types.Bool    `tfsdk:"ha_auto_rebalance"`
	HaAutoRebalanceHoldDuration types.Float64 `tfsdk:"ha_auto_rebalance_hold_duration"`
	HaAutoRebalanceMargin       types.Float64 `tfsdk:"ha_auto_rebalance_margin"`
	HaAutoRebalanceMethod       types.String  `tfsdk:"ha_auto_rebalance_method"`
	HaAutoRebalanceThreshold    types.Float64 `tfsdk:"ha_auto_rebalance_threshold"`
	HaRebalanceOnStart          types.Bool    `tfsdk:"ha_rebalance_on_start"`
}

// pveClusterOptionsHAModel mirrors the ha property string.
type pveClusterOptionsHAModel struct {
	ShutdownPolicy types.String `tfsdk:"shutdown_policy"`
}

// pveClusterOptionsLocationModel mirrors the location property string.
type pveClusterOptionsLocationModel struct {
	Latitude  types.Float64 `tfsdk:"latitude"`
	Longitude types.Float64 `tfsdk:"longitude"`
	Name      types.String  `tfsdk:"name"`
}

// pveClusterOptionsMigrationModel mirrors the migration property string.
type pveClusterOptionsMigrationModel struct {
	Type    types.String `tfsdk:"type"`
	Network types.String `tfsdk:"network"`
}

// pveClusterOptionsNextIDModel mirrors the next-id property string.
type pveClusterOptionsNextIDModel struct {
	Lower types.Int64 `tfsdk:"lower"`
	Upper types.Int64 `tfsdk:"upper"`
}

// pveClusterOptionsNotifyModel mirrors the notify property string.
type pveClusterOptionsNotifyModel struct {
	Fencing              types.String `tfsdk:"fencing"`
	PackageUpdates       types.String `tfsdk:"package_updates"`
	Replication          types.String `tfsdk:"replication"`
	TargetFencing        types.String `tfsdk:"target_fencing"`
	TargetPackageUpdates types.String `tfsdk:"target_package_updates"`
	TargetReplication    types.String `tfsdk:"target_replication"`
}

// pveClusterOptionsReplicationModel mirrors the replication property string.
type pveClusterOptionsReplicationModel struct {
	Type    types.String `tfsdk:"type"`
	Network types.String `tfsdk:"network"`
}

// pveClusterOptionsTagStyleModel mirrors the tag-style property string.
type pveClusterOptionsTagStyleModel struct {
	CaseSensitive types.Bool   `tfsdk:"case_sensitive"`
	ColorMap      types.String `tfsdk:"color_map"`
	Ordering      types.String `tfsdk:"ordering"`
	Shape         types.String `tfsdk:"shape"`
}

// pveClusterOptionsU2FModel mirrors the u2f property string.
type pveClusterOptionsU2FModel struct {
	AppID  types.String `tfsdk:"appid"`
	Origin types.String `tfsdk:"origin"`
}

// pveClusterOptionsUserTagAccessModel mirrors the user-tag-access string.
type pveClusterOptionsUserTagAccessModel struct {
	UserAllow     types.String `tfsdk:"user_allow"`
	UserAllowList types.String `tfsdk:"user_allow_list"`
}

// pveClusterOptionsWebAuthnModel mirrors the webauthn property string.
type pveClusterOptionsWebAuthnModel struct {
	AllowSubdomains types.Bool   `tfsdk:"allow_subdomains"`
	ID              types.String `tfsdk:"id"`
	Origin          types.String `tfsdk:"origin"`
	RP              types.String `tfsdk:"rp"`
}

// Metadata implements resource.Resource.
func (r *pveClusterOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterOptions
}

// Schema implements resource.Resource.
func (r *pveClusterOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the datacenter-wide cluster options singleton (`GET/PUT /cluster/options`). Every listed attribute is managed: removing an attribute from configuration clears the option on the cluster, and removing an inner key of a grouped option rewrites its property string, because PVE stores grouped options atomically. The singleton has no upstream delete verb, so destroy only forgets the state. Requires `Sys.Modify` on `/`.",
		Attributes:          clusterOptionsResourceAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveClusterOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveClusterOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveClusterOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateClusterOptions(ctx, clusterOptionsFromModel(&plan.pveClusterOptionsOptionSet), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_cluster_options",
			fmt.Sprintf("writing cluster options: %s", err),
		)
		return
	}
	if err := clusterOptionsReadInto(ctx, r.client, &plan.pveClusterOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_options after create",
			fmt.Sprintf("reading cluster options: %s", err),
		)
		return
	}
	plan.ID = types.StringValue(pveClusterOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveClusterOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveClusterOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := clusterOptionsReadInto(ctx, r.client, &state.pveClusterOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_options",
			fmt.Sprintf("reading cluster options: %s", err),
		)
		return
	}
	state.ID = types.StringValue(pveClusterOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// fields cleared in the plan travel in the `delete` query parameter.
func (r *pveClusterOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveClusterOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveClusterOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := clusterOptionsDeleteFields(&plan.pveClusterOptionsOptionSet, &state.pveClusterOptionsOptionSet)
	if err := r.client.UpdateClusterOptions(ctx, clusterOptionsFromModel(&plan.pveClusterOptionsOptionSet), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_cluster_options",
			fmt.Sprintf("updating cluster options: %s", err),
		)
		return
	}
	if err := clusterOptionsReadInto(ctx, r.client, &plan.pveClusterOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_options after update",
			fmt.Sprintf("reading cluster options: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The datacenter options singleton has
// no upstream delete verb, so destroy only forgets the state.
func (r *pveClusterOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState adopts the singleton; the identifier is always "cluster",
// whatever ID the import statement supplied.
func (r *pveClusterOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), pveClusterOptionsID)...)
}

// clusterOptionsReadInto refreshes the shared option set from the cluster.
func clusterOptionsReadInto(ctx context.Context, client *pveclient.Client, m *pveClusterOptionsOptionSet) error {
	opts, err := client.GetClusterOptions(ctx)
	if err != nil {
		return fmt.Errorf("reading cluster options: %w", err)
	}
	clusterOptionsApply(m, opts)
	return nil
}

// clusterOptionsFieldKind enumerates the leaf shapes under
// /cluster/options.
type clusterOptionsFieldKind int

const (
	clusterOptionsKindString clusterOptionsFieldKind = iota
	clusterOptionsKindBool
	clusterOptionsKindInt64
	clusterOptionsKindFloat64
	clusterOptionsKindObject
)

// clusterOptionsField describes one /cluster/options attribute for schema
// rendering on both the resource (Optional) and the data source (Computed);
// Wire is the PVE parameter name used in the JSON body and delete list.
type clusterOptionsField struct {
	Name        string
	Wire        string
	Description string
	Kind        clusterOptionsFieldKind
	Enum        []string
	Min, Max    *float64
	Fields      []clusterOptionsField
}

// clusterOptionsFieldSpecs is the full /cluster/options attribute set, in
// wire order, transcribed from the api-spec pin (GET/PUT /cluster/options).
var clusterOptionsFieldSpecs = []clusterOptionsField{
	{Name: "bwlimit", Wire: "bwlimit", Kind: clusterOptionsKindObject, Description: "Cluster-wide I/O bandwidth limits for various operations, in KiB/s.", Fields: []clusterOptionsField{
		{Name: "clone", Wire: "clone", Kind: clusterOptionsKindFloat64, Description: "Bandwidth limit in KiB/s for cloning disks.", Min: clusterOptionsF64(0)},
		{Name: "default", Wire: "default", Kind: clusterOptionsKindFloat64, Description: "Default bandwidth limit in KiB/s.", Min: clusterOptionsF64(0)},
		{Name: "migration", Wire: "migration", Kind: clusterOptionsKindFloat64, Description: "Bandwidth limit in KiB/s for migrating guests, including moving local disks.", Min: clusterOptionsF64(0)},
		{Name: "move", Wire: "move", Kind: clusterOptionsKindFloat64, Description: "Bandwidth limit in KiB/s for moving disks.", Min: clusterOptionsF64(0)},
		{Name: "restore", Wire: "restore", Kind: clusterOptionsKindFloat64, Description: "Bandwidth limit in KiB/s for restoring guests from backups.", Min: clusterOptionsF64(0)},
	}},
	{Name: "consent_text", Wire: "consent-text", Kind: clusterOptionsKindString, Description: "Consent text displayed before logging in, up to 65536 characters."},
	{Name: "console", Wire: "console", Kind: clusterOptionsKindString, Enum: []string{"applet", "vv", "html5", "xtermjs"}, Description: "Default console viewer. If the selected viewer is not available (e.g. SPICE is not activated for the VM), the fallback is noVNC."},
	{Name: "crs", Wire: "crs", Kind: clusterOptionsKindObject, Description: "Cluster resource scheduling (CRS) settings.", Fields: []clusterOptionsField{
		{Name: "ha", Wire: "ha", Kind: clusterOptionsKindString, Enum: []string{"basic", "static", "dynamic"}, Description: "Resource scheduler mode for HA: `basic` counts services, `static` also considers their CPU and memory configuration, `dynamic` also considers current usage."},
		{Name: "ha_auto_rebalance", Wire: "ha-auto-rebalance", Kind: clusterOptionsKindBool, Description: "Whether to use CRS for balancing HA resources automatically depending on the current node imbalance."},
		{Name: "ha_auto_rebalance_hold_duration", Wire: "ha-auto-rebalance-hold-duration", Kind: clusterOptionsKindFloat64, Description: "Number of HA rounds the imbalance threshold must be exceeded before triggering an automatic balancing migration.", Min: clusterOptionsF64(0)},
		{Name: "ha_auto_rebalance_margin", Wire: "ha-auto-rebalance-margin", Kind: clusterOptionsKindFloat64, Description: "Minimum relative improvement in cluster node imbalance, in percent, to commit to a balancing migration.", Min: clusterOptionsF64(0), Max: clusterOptionsF64(100)},
		{Name: "ha_auto_rebalance_method", Wire: "ha-auto-rebalance-method", Kind: clusterOptionsKindString, Enum: []string{"bruteforce", "topsis"}, Description: "Scoring method used for balancing migrations."},
		{Name: "ha_auto_rebalance_threshold", Wire: "ha-auto-rebalance-threshold", Kind: clusterOptionsKindFloat64, Description: "Cluster node imbalance, in percent, that triggers the automatic resource balancing system.", Min: clusterOptionsF64(0), Max: clusterOptionsF64(100)},
		{Name: "ha_rebalance_on_start", Wire: "ha-rebalance-on-start", Kind: clusterOptionsKindBool, Description: "Whether to use CRS for selecting a suited node when an HA service request-state changes from stop to start."},
	}},
	{Name: "description", Wire: "description", Kind: clusterOptionsKindString, Description: "Datacenter description, shown in the web-interface datacenter notes panel, up to 65536 characters."},
	{Name: "email_from", Wire: "email_from", Kind: clusterOptionsKindString, Description: "Email address notifications are sent from (default is root@$hostname)."},
	{Name: "fencing", Wire: "fencing", Kind: clusterOptionsKindString, Enum: []string{"watchdog", "hardware", "both"}, Description: "Fencing mode of the HA cluster. `hardware` needs a valid configuration of fence devices in /etc/pve/ha/fence.cfg; `hardware` and `both` are experimental."},
	{Name: "ha", Wire: "ha", Kind: clusterOptionsKindObject, Description: "Cluster-wide HA settings.", Fields: []clusterOptionsField{
		{Name: "shutdown_policy", Wire: "shutdown_policy", Kind: clusterOptionsKindString, Enum: []string{"freeze", "failover", "conditional", "migrate"}, Description: "Policy for HA services on node shutdown: `freeze` disables auto-recovery, `failover` ensures recovery, `conditional` recovers on poweroff and freezes on reboot, `migrate` moves running services to other nodes if possible."},
	}},
	{Name: "http_proxy", Wire: "http_proxy", Kind: clusterOptionsKindString, Description: "External HTTP proxy used for downloads, e.g. `http://username:password@host:port/`."},
	{Name: "keyboard", Wire: "keyboard", Kind: clusterOptionsKindString, Enum: []string{"de", "de-ch", "da", "en-gb", "en-us", "es", "fi", "fr", "fr-be", "fr-ca", "fr-ch", "hu", "is", "it", "ja", "lt", "mk", "nl", "no", "pl", "pt", "pt-br", "sv", "sl", "tr"}, Description: "Default keyboard layout for the VNC server."},
	{Name: "language", Wire: "language", Kind: clusterOptionsKindString, Enum: []string{"ar", "ca", "da", "de", "en", "es", "eu", "fa", "fr", "hr", "he", "it", "ja", "ka", "kr", "nb", "nl", "nn", "pl", "pt_BR", "ru", "sl", "sv", "tr", "ukr", "zh_CN", "zh_TW"}, Description: "Default GUI language."},
	{Name: "location", Wire: "location", Kind: clusterOptionsKindObject, Description: "Physical location of the cluster.", Fields: []clusterOptionsField{
		{Name: "latitude", Wire: "latitude", Kind: clusterOptionsKindFloat64, Description: "Latitude of the cluster location."},
		{Name: "longitude", Wire: "longitude", Kind: clusterOptionsKindFloat64, Description: "Longitude of the cluster location."},
		{Name: "name", Wire: "name", Kind: clusterOptionsKindString, Description: "Name of the cluster location."},
	}},
	{Name: "mac_prefix", Wire: "mac_prefix", Kind: clusterOptionsKindString, Description: "Prefix for auto-generated guest MAC addresses (default `BC:24:11`, the Proxmox OUI)."},
	{Name: "max_workers", Wire: "max_workers", Kind: clusterOptionsKindInt64, Description: "Maximum number of workers per node started for actions like stopping all VMs.", Min: clusterOptionsF64(1)},
	{Name: "migration", Wire: "migration", Kind: clusterOptionsKindObject, Description: "Cluster-wide migration settings.", Fields: []clusterOptionsField{
		{Name: "type", Wire: "type", Kind: clusterOptionsKindString, Enum: []string{"secure", "insecure"}, Description: "Migration traffic is encrypted using an SSH tunnel by default; on completely private networks this can be disabled to increase performance."},
		{Name: "network", Wire: "network", Kind: clusterOptionsKindString, Description: "CIDR of the (sub)network used for migration; used as a fallback for replication jobs when the replication network setting is not set."},
	}},
	{Name: "migration_unsecure", Wire: "migration_unsecure", Kind: clusterOptionsKindBool, Description: "Deprecated: whether migration may run without the SSH tunnel; use `migration.type` instead."},
	{Name: "next_id", Wire: "next-id", Kind: clusterOptionsKindObject, Description: "Range control for the free VMID auto-selection pool.", Fields: []clusterOptionsField{
		{Name: "lower", Wire: "lower", Kind: clusterOptionsKindInt64, Description: "Lower, inclusive boundary of the free next-id API range.", Min: clusterOptionsF64(100), Max: clusterOptionsF64(999999999)},
		{Name: "upper", Wire: "upper", Kind: clusterOptionsKindInt64, Description: "Upper, exclusive boundary of the free next-id API range.", Min: clusterOptionsF64(100), Max: clusterOptionsF64(1000000000)},
	}},
	{Name: "notify", Wire: "notify", Kind: clusterOptionsKindObject, Description: "Cluster-wide notification settings; several keys are deprecated in favor of the notification endpoint and matcher resources.", Fields: []clusterOptionsField{
		{Name: "fencing", Wire: "fencing", Kind: clusterOptionsKindString, Enum: []string{"always", "never"}, Description: "Unused: use datacenter notification settings instead."},
		{Name: "package_updates", Wire: "package-updates", Kind: clusterOptionsKindString, Enum: []string{"auto", "always", "never"}, Description: "Deprecated: when the daily update job should send notifications; `auto` means daily for systems with a valid subscription."},
		{Name: "replication", Wire: "replication", Kind: clusterOptionsKindString, Enum: []string{"always", "never"}, Description: "Unused: use datacenter notification settings instead."},
		{Name: "target_fencing", Wire: "target-fencing", Kind: clusterOptionsKindString, Description: "Unused: notification target for fencing events."},
		{Name: "target_package_updates", Wire: "target-package-updates", Kind: clusterOptionsKindString, Description: "Unused: notification target for package update events."},
		{Name: "target_replication", Wire: "target-replication", Kind: clusterOptionsKindString, Description: "Unused: notification target for replication events."},
	}},
	{Name: "registered_tags", Wire: "registered-tags", Kind: clusterOptionsKindString, Description: "Semicolon-separated list of tags that require `Sys.Modify` on `/` to set or delete."},
	{Name: "replication", Wire: "replication", Kind: clusterOptionsKindObject, Description: "Cluster-wide replication settings.", Fields: []clusterOptionsField{
		{Name: "type", Wire: "type", Kind: clusterOptionsKindString, Enum: []string{"secure", "insecure"}, Description: "Replication traffic is encrypted using an SSH tunnel by default; on completely private networks this can be disabled to increase performance."},
		{Name: "network", Wire: "network", Kind: clusterOptionsKindString, Description: "CIDR of the (sub)network used for replication jobs."},
	}},
	{Name: "tag_style", Wire: "tag-style", Kind: clusterOptionsKindObject, Description: "Tag style options for the web interface.", Fields: []clusterOptionsField{
		{Name: "case_sensitive", Wire: "case-sensitive", Kind: clusterOptionsKindBool, Description: "Whether filtering for unique tags on update should check case-sensitively."},
		{Name: "color_map", Wire: "color-map", Kind: clusterOptionsKindString, Description: "Semicolon-separated manual color mapping for tags, each as `tag:hex-color[:hex-color-for-text]`."},
		{Name: "ordering", Wire: "ordering", Kind: clusterOptionsKindString, Enum: []string{"config", "alphabetical"}, Description: "Sorting of tags in the web interface and the API update."},
		{Name: "shape", Wire: "shape", Kind: clusterOptionsKindString, Enum: []string{"full", "circle", "dense", "none"}, Description: "Tag shape in the web interface tree."},
	}},
	{Name: "u2f", Wire: "u2f", Kind: clusterOptionsKindObject, Description: "U2F configuration.", Fields: []clusterOptionsField{
		{Name: "appid", Wire: "appid", Kind: clusterOptionsKindString, Description: "U2F AppId URL override; defaults to the origin."},
		{Name: "origin", Wire: "origin", Kind: clusterOptionsKindString, Description: "U2F origin override, mostly useful for single nodes with a single URL."},
	}},
	{Name: "user_tag_access", Wire: "user-tag-access", Kind: clusterOptionsKindObject, Description: "Privilege options for user-settable tags.", Fields: []clusterOptionsField{
		{Name: "user_allow", Wire: "user-allow", Kind: clusterOptionsKindString, Enum: []string{"none", "list", "existing", "free"}, Description: "Controls tag usage for users without `Sys.Modify` on `/`: `none` allows nothing, `list` allows `user_allow_list` tags, `existing` also allows already-set tags, `free` imposes no restrictions."},
		{Name: "user_allow_list", Wire: "user-allow-list", Kind: clusterOptionsKindString, Description: "Semicolon-separated list of tags users may set and delete for the `list` and `existing` `user_allow` values."},
	}},
	{Name: "webauthn", Wire: "webauthn", Kind: clusterOptionsKindObject, Description: "WebAuthn configuration.", Fields: []clusterOptionsField{
		{Name: "allow_subdomains", Wire: "allow-subdomains", Kind: clusterOptionsKindBool, Description: "Whether the origin may be a subdomain rather than the exact URL."},
		{Name: "id", Wire: "id", Kind: clusterOptionsKindString, Description: "Relying party ID: the domain name without protocol, port or location. Changing this breaks existing credentials."},
		{Name: "origin", Wire: "origin", Kind: clusterOptionsKindString, Description: "Site origin as a `https://` URL (or `http://localhost`); changing this may break existing credentials."},
		{Name: "rp", Wire: "rp", Kind: clusterOptionsKindString, Description: "Relying party name; changing this may break existing credentials."},
	}},
}

// clusterOptionsF64 returns a pointer to v, used for spec-table bounds.
func clusterOptionsF64(v float64) *float64 { return &v }

// clusterOptionsEnumText renders the inline enumeration required in every
// closed-set attribute description.
func clusterOptionsEnumText(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, "`"+v+"`")
	}
	return "Must be one of: " + strings.Join(quoted, ", ") + "."
}

// clusterOptionsBoundsText renders the numeric range for descriptions.
func clusterOptionsBoundsText(f clusterOptionsField) string {
	low, high := "", ""
	if f.Min != nil {
		low = strconv.FormatFloat(*f.Min, 'f', -1, 64)
	}
	if f.Max != nil {
		high = strconv.FormatFloat(*f.Max, 'f', -1, 64)
	}
	switch {
	case f.Min != nil && f.Max != nil:
		return "Must be between " + low + " and " + high + "."
	case f.Min != nil:
		return "Must be at least " + low + "."
	case f.Max != nil:
		return "Must be at most " + high + "."
	default:
		return ""
	}
}

// clusterOptionsLeafDescription composes the full Markdown description:
// base text plus the required inline enumeration of closed sets and numeric
// ranges.
func clusterOptionsLeafDescription(f clusterOptionsField) string {
	parts := []string{f.Description}
	if len(f.Enum) > 0 {
		parts = append(parts, clusterOptionsEnumText(f.Enum))
	}
	if text := clusterOptionsBoundsText(f); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

// clusterOptionsStringValidators renders OneOf for closed-set strings.
func clusterOptionsStringValidators(f clusterOptionsField) []validator.String {
	if len(f.Enum) == 0 {
		return nil
	}
	return []validator.String{stringvalidator.OneOf(f.Enum...)}
}

// clusterOptionsInt64Validators renders the pin's numeric range.
func clusterOptionsInt64Validators(f clusterOptionsField) []validator.Int64 {
	switch {
	case f.Min != nil && f.Max != nil:
		return []validator.Int64{int64validator.Between(int64(*f.Min), int64(*f.Max))}
	case f.Min != nil:
		return []validator.Int64{int64validator.AtLeast(int64(*f.Min))}
	case f.Max != nil:
		return []validator.Int64{int64validator.AtMost(int64(*f.Max))}
	default:
		return nil
	}
}

// clusterOptionsFloat64Validators renders the pin's numeric range.
func clusterOptionsFloat64Validators(f clusterOptionsField) []validator.Float64 {
	switch {
	case f.Min != nil && f.Max != nil:
		return []validator.Float64{float64validator.Between(*f.Min, *f.Max)}
	case f.Min != nil:
		return []validator.Float64{float64validator.AtLeast(*f.Min)}
	case f.Max != nil:
		return []validator.Float64{float64validator.AtMost(*f.Max)}
	default:
		return nil
	}
}

// clusterOptionsResourceLeaf renders one spec entry as a resource
// attribute.
func clusterOptionsResourceLeaf(f clusterOptionsField) schema.Attribute {
	switch f.Kind {
	case clusterOptionsKindBool:
		return schema.BoolAttribute{Optional: true, MarkdownDescription: clusterOptionsLeafDescription(f)}
	case clusterOptionsKindInt64:
		return schema.Int64Attribute{Optional: true, MarkdownDescription: clusterOptionsLeafDescription(f), Validators: clusterOptionsInt64Validators(f)}
	case clusterOptionsKindFloat64:
		return schema.Float64Attribute{Optional: true, MarkdownDescription: clusterOptionsLeafDescription(f), Validators: clusterOptionsFloat64Validators(f)}
	case clusterOptionsKindObject:
		attrs := make(map[string]schema.Attribute, len(f.Fields))
		for _, inner := range f.Fields {
			attrs[inner.Name] = clusterOptionsResourceLeaf(inner)
		}
		return schema.SingleNestedAttribute{Optional: true, MarkdownDescription: f.Description, Attributes: attrs}
	default:
		return schema.StringAttribute{Optional: true, MarkdownDescription: clusterOptionsLeafDescription(f), Validators: clusterOptionsStringValidators(f)}
	}
}

// clusterOptionsResourceAttributes renders the full resource attribute set.
func clusterOptionsResourceAttributes() map[string]schema.Attribute {
	attrs := make(map[string]schema.Attribute, len(clusterOptionsFieldSpecs)+1)
	for _, f := range clusterOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsResourceLeaf(f)
	}
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Singleton identifier for the datacenter options; always `cluster`.",
	}
	return attrs
}

// clusterOptionsStringCleared reports whether a string attribute is set in
// state but null in the plan, i.e. must be cleared upstream.
func clusterOptionsStringCleared(plan, state types.String) bool {
	return plan.IsNull() && !state.IsNull()
}

// clusterOptionsDeleteFields returns the wire names of options present in
// state but cleared in the plan. Grouped options delete as a whole because
// PVE property strings are atomic.
func clusterOptionsDeleteFields(plan, state *pveClusterOptionsOptionSet) []string {
	var out []string
	if plan.BWLimit == nil && state.BWLimit != nil {
		out = append(out, "bwlimit")
	}
	if clusterOptionsStringCleared(plan.ConsentText, state.ConsentText) {
		out = append(out, "consent-text")
	}
	if clusterOptionsStringCleared(plan.Console, state.Console) {
		out = append(out, "console")
	}
	if plan.CRS == nil && state.CRS != nil {
		out = append(out, "crs")
	}
	if clusterOptionsStringCleared(plan.Description, state.Description) {
		out = append(out, "description")
	}
	if clusterOptionsStringCleared(plan.EmailFrom, state.EmailFrom) {
		out = append(out, "email_from")
	}
	if clusterOptionsStringCleared(plan.Fencing, state.Fencing) {
		out = append(out, "fencing")
	}
	if plan.HA == nil && state.HA != nil {
		out = append(out, "ha")
	}
	if clusterOptionsStringCleared(plan.HTTPProxy, state.HTTPProxy) {
		out = append(out, "http_proxy")
	}
	if clusterOptionsStringCleared(plan.Keyboard, state.Keyboard) {
		out = append(out, "keyboard")
	}
	if clusterOptionsStringCleared(plan.Language, state.Language) {
		out = append(out, "language")
	}
	if plan.Location == nil && state.Location != nil {
		out = append(out, "location")
	}
	if clusterOptionsStringCleared(plan.MACPrefix, state.MACPrefix) {
		out = append(out, "mac_prefix")
	}
	if plan.MaxWorkers.IsNull() && !state.MaxWorkers.IsNull() {
		out = append(out, "max_workers")
	}
	if plan.Migration == nil && state.Migration != nil {
		out = append(out, "migration")
	}
	if plan.MigrationUnsecure.IsNull() && !state.MigrationUnsecure.IsNull() {
		out = append(out, "migration_unsecure")
	}
	if plan.NextID == nil && state.NextID != nil {
		out = append(out, "next-id")
	}
	if plan.Notify == nil && state.Notify != nil {
		out = append(out, "notify")
	}
	if clusterOptionsStringCleared(plan.RegisteredTags, state.RegisteredTags) {
		out = append(out, "registered-tags")
	}
	if plan.Replication == nil && state.Replication != nil {
		out = append(out, "replication")
	}
	if plan.TagStyle == nil && state.TagStyle != nil {
		out = append(out, "tag-style")
	}
	if plan.U2F == nil && state.U2F != nil {
		out = append(out, "u2f")
	}
	if plan.UserTagAccess == nil && state.UserTagAccess != nil {
		out = append(out, "user-tag-access")
	}
	if plan.WebAuthn == nil && state.WebAuthn != nil {
		out = append(out, "webauthn")
	}
	return out
}

// clusterOptionsStrPtr converts a Terraform string into a wire pointer,
// nil for null or unknown values.
func clusterOptionsStrPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// clusterOptionsBoolPtr converts a Terraform bool into a wire pointer.
func clusterOptionsBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// clusterOptionsIntPtr converts a Terraform int64 into a wire pointer.
func clusterOptionsIntPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

// clusterOptionsFloatPtr converts a Terraform float64 into a wire pointer.
func clusterOptionsFloatPtr(v types.Float64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := v.ValueFloat64()
	return &f
}

// clusterOptionsStringValue writes a wire string into the model, null when
// absent.
func clusterOptionsStringValue(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// clusterOptionsBoolValue writes a wire bool into the model.
func clusterOptionsBoolValue(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// clusterOptionsInt64Value writes a wire int into the model.
func clusterOptionsInt64Value(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// clusterOptionsFloat64Value writes a wire float into the model.
func clusterOptionsFloat64Value(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

// clusterOptionsFromModel projects the Terraform model into the wire
// struct; null values become nil pointers and are omitted from the PUT body.
func clusterOptionsFromModel(m *pveClusterOptionsOptionSet) pveclient.ClusterOptions {
	return pveclient.ClusterOptions{
		BWLimit:           clusterOptionsBWLimitFromModel(m.BWLimit),
		ConsentText:       clusterOptionsStrPtr(m.ConsentText),
		Console:           clusterOptionsStrPtr(m.Console),
		CRS:               clusterOptionsCRSFromModel(m.CRS),
		Description:       clusterOptionsStrPtr(m.Description),
		EmailFrom:         clusterOptionsStrPtr(m.EmailFrom),
		Fencing:           clusterOptionsStrPtr(m.Fencing),
		HA:                clusterOptionsHAFromModel(m.HA),
		HTTPProxy:         clusterOptionsStrPtr(m.HTTPProxy),
		Keyboard:          clusterOptionsStrPtr(m.Keyboard),
		Language:          clusterOptionsStrPtr(m.Language),
		Location:          clusterOptionsLocationFromModel(m.Location),
		MACPrefix:         clusterOptionsStrPtr(m.MACPrefix),
		MaxWorkers:        clusterOptionsIntPtr(m.MaxWorkers),
		Migration:         clusterOptionsMigrationFromModel(m.Migration),
		MigrationUnsecure: clusterOptionsBoolPtr(m.MigrationUnsecure),
		NextID:            clusterOptionsNextIDFromModel(m.NextID),
		Notify:            clusterOptionsNotifyFromModel(m.Notify),
		RegisteredTags:    clusterOptionsStrPtr(m.RegisteredTags),
		Replication:       clusterOptionsReplicationFromModel(m.Replication),
		TagStyle:          clusterOptionsTagStyleFromModel(m.TagStyle),
		U2F:               clusterOptionsU2FFromModel(m.U2F),
		UserTagAccess:     clusterOptionsUserTagAccessFromModel(m.UserTagAccess),
		WebAuthn:          clusterOptionsWebAuthnFromModel(m.WebAuthn),
	}
}

// clusterOptionsBWLimitFromModel projects the bwlimit object.
func clusterOptionsBWLimitFromModel(m *pveClusterOptionsBWLimitModel) *pveclient.ClusterOptionsBWLimit {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsBWLimit{
		Clone:     clusterOptionsFloatPtr(m.Clone),
		Default:   clusterOptionsFloatPtr(m.Default),
		Migration: clusterOptionsFloatPtr(m.Migration),
		Move:      clusterOptionsFloatPtr(m.Move),
		Restore:   clusterOptionsFloatPtr(m.Restore),
	}
}

// clusterOptionsCRSFromModel projects the crs object.
func clusterOptionsCRSFromModel(m *pveClusterOptionsCRSModel) *pveclient.ClusterOptionsCRS {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsCRS{
		Ha:                          clusterOptionsStrPtr(m.Ha),
		HaAutoRebalance:             clusterOptionsBoolPtr(m.HaAutoRebalance),
		HaAutoRebalanceHoldDuration: clusterOptionsFloatPtr(m.HaAutoRebalanceHoldDuration),
		HaAutoRebalanceMargin:       clusterOptionsFloatPtr(m.HaAutoRebalanceMargin),
		HaAutoRebalanceMethod:       clusterOptionsStrPtr(m.HaAutoRebalanceMethod),
		HaAutoRebalanceThreshold:    clusterOptionsFloatPtr(m.HaAutoRebalanceThreshold),
		HaRebalanceOnStart:          clusterOptionsBoolPtr(m.HaRebalanceOnStart),
	}
}

// clusterOptionsHAFromModel projects the ha object.
func clusterOptionsHAFromModel(m *pveClusterOptionsHAModel) *pveclient.ClusterOptionsHA {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsHA{ShutdownPolicy: clusterOptionsStrPtr(m.ShutdownPolicy)}
}

// clusterOptionsLocationFromModel projects the location object.
func clusterOptionsLocationFromModel(m *pveClusterOptionsLocationModel) *pveclient.ClusterOptionsLocation {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsLocation{
		Latitude:  clusterOptionsFloatPtr(m.Latitude),
		Longitude: clusterOptionsFloatPtr(m.Longitude),
		Name:      clusterOptionsStrPtr(m.Name),
	}
}

// clusterOptionsMigrationFromModel projects the migration object.
func clusterOptionsMigrationFromModel(m *pveClusterOptionsMigrationModel) *pveclient.ClusterOptionsMigration {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsMigration{
		Type:    clusterOptionsStrPtr(m.Type),
		Network: clusterOptionsStrPtr(m.Network),
	}
}

// clusterOptionsNextIDFromModel projects the next_id object.
func clusterOptionsNextIDFromModel(m *pveClusterOptionsNextIDModel) *pveclient.ClusterOptionsNextID {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsNextID{
		Lower: clusterOptionsIntPtr(m.Lower),
		Upper: clusterOptionsIntPtr(m.Upper),
	}
}

// clusterOptionsNotifyFromModel projects the notify object.
func clusterOptionsNotifyFromModel(m *pveClusterOptionsNotifyModel) *pveclient.ClusterOptionsNotify {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsNotify{
		Fencing:              clusterOptionsStrPtr(m.Fencing),
		PackageUpdates:       clusterOptionsStrPtr(m.PackageUpdates),
		Replication:          clusterOptionsStrPtr(m.Replication),
		TargetFencing:        clusterOptionsStrPtr(m.TargetFencing),
		TargetPackageUpdates: clusterOptionsStrPtr(m.TargetPackageUpdates),
		TargetReplication:    clusterOptionsStrPtr(m.TargetReplication),
	}
}

// clusterOptionsReplicationFromModel projects the replication object.
func clusterOptionsReplicationFromModel(m *pveClusterOptionsReplicationModel) *pveclient.ClusterOptionsReplication {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsReplication{
		Type:    clusterOptionsStrPtr(m.Type),
		Network: clusterOptionsStrPtr(m.Network),
	}
}

// clusterOptionsTagStyleFromModel projects the tag_style object.
func clusterOptionsTagStyleFromModel(m *pveClusterOptionsTagStyleModel) *pveclient.ClusterOptionsTagStyle {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsTagStyle{
		CaseSensitive: clusterOptionsBoolPtr(m.CaseSensitive),
		ColorMap:      clusterOptionsStrPtr(m.ColorMap),
		Ordering:      clusterOptionsStrPtr(m.Ordering),
		Shape:         clusterOptionsStrPtr(m.Shape),
	}
}

// clusterOptionsU2FFromModel projects the u2f object.
func clusterOptionsU2FFromModel(m *pveClusterOptionsU2FModel) *pveclient.ClusterOptionsU2F {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsU2F{
		AppID:  clusterOptionsStrPtr(m.AppID),
		Origin: clusterOptionsStrPtr(m.Origin),
	}
}

// clusterOptionsUserTagAccessFromModel projects the user_tag_access object.
func clusterOptionsUserTagAccessFromModel(m *pveClusterOptionsUserTagAccessModel) *pveclient.ClusterOptionsUserTagAccess {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsUserTagAccess{
		UserAllow:     clusterOptionsStrPtr(m.UserAllow),
		UserAllowList: clusterOptionsStrPtr(m.UserAllowList),
	}
}

// clusterOptionsWebAuthnFromModel projects the webauthn object.
func clusterOptionsWebAuthnFromModel(m *pveClusterOptionsWebAuthnModel) *pveclient.ClusterOptionsWebAuthn {
	if m == nil {
		return nil
	}
	return &pveclient.ClusterOptionsWebAuthn{
		AllowSubdomains: clusterOptionsBoolPtr(m.AllowSubdomains),
		ID:              clusterOptionsStrPtr(m.ID),
		Origin:          clusterOptionsStrPtr(m.Origin),
		RP:              clusterOptionsStrPtr(m.RP),
	}
}

// clusterOptionsApply writes fetched options into the model; absent options
// and keys become null.
func clusterOptionsApply(m *pveClusterOptionsOptionSet, o *pveclient.ClusterOptions) {
	m.BWLimit = clusterOptionsBWLimitApply(o.BWLimit)
	m.ConsentText = clusterOptionsStringValue(o.ConsentText)
	m.Console = clusterOptionsStringValue(o.Console)
	m.CRS = clusterOptionsCRSApply(o.CRS)
	m.Description = clusterOptionsStringValue(o.Description)
	m.EmailFrom = clusterOptionsStringValue(o.EmailFrom)
	m.Fencing = clusterOptionsStringValue(o.Fencing)
	m.HA = clusterOptionsHAApply(o.HA)
	m.HTTPProxy = clusterOptionsStringValue(o.HTTPProxy)
	m.Keyboard = clusterOptionsStringValue(o.Keyboard)
	m.Language = clusterOptionsStringValue(o.Language)
	m.Location = clusterOptionsLocationApply(o.Location)
	m.MACPrefix = clusterOptionsStringValue(o.MACPrefix)
	m.MaxWorkers = clusterOptionsInt64Value(o.MaxWorkers)
	m.Migration = clusterOptionsMigrationApply(o.Migration)
	m.MigrationUnsecure = clusterOptionsBoolValue(o.MigrationUnsecure)
	m.NextID = clusterOptionsNextIDApply(o.NextID)
	m.Notify = clusterOptionsNotifyApply(o.Notify)
	m.RegisteredTags = clusterOptionsStringValue(o.RegisteredTags)
	m.Replication = clusterOptionsReplicationApply(o.Replication)
	m.TagStyle = clusterOptionsTagStyleApply(o.TagStyle)
	m.U2F = clusterOptionsU2FApply(o.U2F)
	m.UserTagAccess = clusterOptionsUserTagAccessApply(o.UserTagAccess)
	m.WebAuthn = clusterOptionsWebAuthnApply(o.WebAuthn)
}

// clusterOptionsBWLimitApply builds the bwlimit object model.
func clusterOptionsBWLimitApply(o *pveclient.ClusterOptionsBWLimit) *pveClusterOptionsBWLimitModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsBWLimitModel{
		Clone:     clusterOptionsFloat64Value(o.Clone),
		Default:   clusterOptionsFloat64Value(o.Default),
		Migration: clusterOptionsFloat64Value(o.Migration),
		Move:      clusterOptionsFloat64Value(o.Move),
		Restore:   clusterOptionsFloat64Value(o.Restore),
	}
}

// clusterOptionsCRSApply builds the crs object model.
func clusterOptionsCRSApply(o *pveclient.ClusterOptionsCRS) *pveClusterOptionsCRSModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsCRSModel{
		Ha:                          clusterOptionsStringValue(o.Ha),
		HaAutoRebalance:             clusterOptionsBoolValue(o.HaAutoRebalance),
		HaAutoRebalanceHoldDuration: clusterOptionsFloat64Value(o.HaAutoRebalanceHoldDuration),
		HaAutoRebalanceMargin:       clusterOptionsFloat64Value(o.HaAutoRebalanceMargin),
		HaAutoRebalanceMethod:       clusterOptionsStringValue(o.HaAutoRebalanceMethod),
		HaAutoRebalanceThreshold:    clusterOptionsFloat64Value(o.HaAutoRebalanceThreshold),
		HaRebalanceOnStart:          clusterOptionsBoolValue(o.HaRebalanceOnStart),
	}
}

// clusterOptionsHAApply builds the ha object model.
func clusterOptionsHAApply(o *pveclient.ClusterOptionsHA) *pveClusterOptionsHAModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsHAModel{ShutdownPolicy: clusterOptionsStringValue(o.ShutdownPolicy)}
}

// clusterOptionsLocationApply builds the location object model.
func clusterOptionsLocationApply(o *pveclient.ClusterOptionsLocation) *pveClusterOptionsLocationModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsLocationModel{
		Latitude:  clusterOptionsFloat64Value(o.Latitude),
		Longitude: clusterOptionsFloat64Value(o.Longitude),
		Name:      clusterOptionsStringValue(o.Name),
	}
}

// clusterOptionsMigrationApply builds the migration object model.
func clusterOptionsMigrationApply(o *pveclient.ClusterOptionsMigration) *pveClusterOptionsMigrationModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsMigrationModel{
		Type:    clusterOptionsStringValue(o.Type),
		Network: clusterOptionsStringValue(o.Network),
	}
}

// clusterOptionsNextIDApply builds the next_id object model.
func clusterOptionsNextIDApply(o *pveclient.ClusterOptionsNextID) *pveClusterOptionsNextIDModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsNextIDModel{
		Lower: clusterOptionsInt64Value(o.Lower),
		Upper: clusterOptionsInt64Value(o.Upper),
	}
}

// clusterOptionsNotifyApply builds the notify object model.
func clusterOptionsNotifyApply(o *pveclient.ClusterOptionsNotify) *pveClusterOptionsNotifyModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsNotifyModel{
		Fencing:              clusterOptionsStringValue(o.Fencing),
		PackageUpdates:       clusterOptionsStringValue(o.PackageUpdates),
		Replication:          clusterOptionsStringValue(o.Replication),
		TargetFencing:        clusterOptionsStringValue(o.TargetFencing),
		TargetPackageUpdates: clusterOptionsStringValue(o.TargetPackageUpdates),
		TargetReplication:    clusterOptionsStringValue(o.TargetReplication),
	}
}

// clusterOptionsReplicationApply builds the replication object model.
func clusterOptionsReplicationApply(o *pveclient.ClusterOptionsReplication) *pveClusterOptionsReplicationModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsReplicationModel{
		Type:    clusterOptionsStringValue(o.Type),
		Network: clusterOptionsStringValue(o.Network),
	}
}

// clusterOptionsTagStyleApply builds the tag_style object model.
func clusterOptionsTagStyleApply(o *pveclient.ClusterOptionsTagStyle) *pveClusterOptionsTagStyleModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsTagStyleModel{
		CaseSensitive: clusterOptionsBoolValue(o.CaseSensitive),
		ColorMap:      clusterOptionsStringValue(o.ColorMap),
		Ordering:      clusterOptionsStringValue(o.Ordering),
		Shape:         clusterOptionsStringValue(o.Shape),
	}
}

// clusterOptionsU2FApply builds the u2f object model.
func clusterOptionsU2FApply(o *pveclient.ClusterOptionsU2F) *pveClusterOptionsU2FModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsU2FModel{
		AppID:  clusterOptionsStringValue(o.AppID),
		Origin: clusterOptionsStringValue(o.Origin),
	}
}

// clusterOptionsUserTagAccessApply builds the user_tag_access object model.
func clusterOptionsUserTagAccessApply(o *pveclient.ClusterOptionsUserTagAccess) *pveClusterOptionsUserTagAccessModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsUserTagAccessModel{
		UserAllow:     clusterOptionsStringValue(o.UserAllow),
		UserAllowList: clusterOptionsStringValue(o.UserAllowList),
	}
}

// clusterOptionsWebAuthnApply builds the webauthn object model.
func clusterOptionsWebAuthnApply(o *pveclient.ClusterOptionsWebAuthn) *pveClusterOptionsWebAuthnModel {
	if o == nil {
		return nil
	}
	return &pveClusterOptionsWebAuthnModel{
		AllowSubdomains: clusterOptionsBoolValue(o.AllowSubdomains),
		ID:              clusterOptionsStringValue(o.ID),
		Origin:          clusterOptionsStringValue(o.Origin),
		RP:              clusterOptionsStringValue(o.RP),
	}
}
