// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveRealmSyncAction{}
	_ action.ActionWithConfigure = &pveRealmSyncAction{}
)

// NewPveRealmSyncAction returns the action implementation.
func NewPveRealmSyncAction() action.Action {
	return &pveRealmSyncAction{}
}

// pveRealmSyncAction syncs users and/or groups of an LDAP/AD realm from
// the upstream directory into user.cfg (POST /access/domains/{realm}/sync).
type pveRealmSyncAction struct {
	client *pveclient.Client
}

// pveRealmSyncActionModel is the Terraform-facing shape.
type pveRealmSyncActionModel struct {
	Realm          types.String `tfsdk:"realm"`
	Scope          types.String `tfsdk:"scope"`
	DryRun         types.Bool   `tfsdk:"dry_run"`
	EnableNew      types.Bool   `tfsdk:"enable_new"`
	RemoveVanished types.String `tfsdk:"remove_vanished"`
	Full           types.Bool   `tfsdk:"full"`
	Purge          types.Bool   `tfsdk:"purge"`
}

// Metadata implements action.Action.
func (a *pveRealmSyncAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmSync
}

// Schema implements action.Action.
func (a *pveRealmSyncAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Syncs users and/or groups from the configured LDAP/AD directory into `user.cfg` (`POST /access/domains/{realm}/sync`) and waits for the sync task to finish. Synced groups are named `name-$realm`, so make sure those groups do not already exist to prevent overwriting.",
		Attributes: map[string]actionschema.Attribute{
			"realm": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Authentication domain ID to sync.",
			},
			"scope": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Select what to sync. Must be one of `users`, `groups`, `both`.",
				Validators: []validator.String{
					stringvalidator.OneOf("users", "groups", "both"),
				},
			},
			"dry_run": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If set, does not write anything.",
			},
			"enable_new": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable newly synced users immediately. Defaults to `true`.",
			},
			"remove_vanished": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A semicolon-separated list of things to remove when they vanish during the sync: `entry` removes the user/group when not returned from the sync, `properties` removes properties of existing users/groups missing in the source, `acl` removes ACLs of vanished users/groups. `none` (the default) disables all removals, e.g. `entry;acl`.",
			},
			"full": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated upstream: use `remove_vanished` instead. When set, the directory is the source of truth: users/groups not returned are deleted and locally modified properties of synced users are reset.",
			},
			"purge": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated upstream: use `remove_vanished` instead. Removes ACLs for users or groups removed from the config during the sync.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveRealmSyncAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	a.client = client
}

// Invoke implements action.Action.
func (a *pveRealmSyncAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveRealmSyncActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_realm_sync",
			fmt.Sprintf("The provider client was not configured; cannot sync realm %s.", config.Realm.ValueString()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	if err := a.run(ctx, config, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_realm_sync",
			fmt.Sprintf("syncing realm %s: %s", config.Realm.ValueString(), err),
		)
	}
}

// run performs the sync request and waits for the worker task.
func (a *pveRealmSyncAction) run(ctx context.Context, config pveRealmSyncActionModel, progress func(string)) error {
	realm := config.Realm.ValueString()
	opts := pveclient.SyncDomainOptions{}
	if !config.Scope.IsNull() && !config.Scope.IsUnknown() {
		opts.Scope = config.Scope.ValueString()
	}
	if !config.DryRun.IsNull() && !config.DryRun.IsUnknown() {
		v := config.DryRun.ValueBool()
		opts.DryRun = &v
	}
	if !config.EnableNew.IsNull() && !config.EnableNew.IsUnknown() {
		v := config.EnableNew.ValueBool()
		opts.EnableNew = &v
	}
	if !config.RemoveVanished.IsNull() && !config.RemoveVanished.IsUnknown() {
		opts.RemoveVanished = config.RemoveVanished.ValueString()
	}
	if !config.Full.IsNull() && !config.Full.IsUnknown() {
		v := config.Full.ValueBool()
		opts.Full = &v
	}
	if !config.Purge.IsNull() && !config.Purge.IsUnknown() {
		v := config.Purge.ValueBool()
		opts.Purge = &v
	}
	tflog.Info(ctx, "starting realm sync", map[string]any{"realm": realm})
	upid, err := a.client.SyncDomain(ctx, realm, opts)
	if err != nil {
		return fmt.Errorf("starting sync: %w", err)
	}
	node, err := realmNodeFromUpid(upid)
	if err != nil {
		return err
	}
	progress(fmt.Sprintf("Realm sync for %s started on node %s; waiting for task to finish", realm, node))
	tflog.Debug(ctx, "waiting for realm sync task", map[string]any{"realm": realm, "upid": upid})
	if _, err := a.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return err
	}
	progress(fmt.Sprintf("Realm sync for %s finished", realm))
	return nil
}

// realmNodeFromUpid extracts the node name from a PVE worker task UPID
// (`UPID:<node>:<pid>:<pstart>:<dtype>:<id>:<user>: `).
func realmNodeFromUpid(upid string) (string, error) {
	parts := strings.Split(upid, ":")
	if len(parts) < 2 || parts[0] != "UPID" || parts[1] == "" {
		return "", fmt.Errorf("cannot determine node from task UPID %q", upid)
	}
	return parts[1], nil
}
