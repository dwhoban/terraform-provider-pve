// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeHostsResource{}
	_ resource.ResourceWithConfigure   = &pveNodeHostsResource{}
	_ resource.ResourceWithImportState = &pveNodeHostsResource{}
)

// NewPveNodeHostsResource returns the /etc/hosts resource implementation.
func NewPveNodeHostsResource() resource.Resource {
	return &pveNodeHostsResource{}
}

// pveNodeHostsResource manages the /etc/hosts file of one node as a
// singleton (GET/POST /nodes/{node}/hosts). Writes replace the whole
// file, so every write re-reads afterwards to pick up hostsd
// normalization. Deleting the resource only removes it from Terraform
// state; PVE has no endpoint to empty /etc/hosts.
type pveNodeHostsResource struct {
	client *pveclient.Client
}

// pveNodeHostsResourceModel is the Terraform-facing shape.
type pveNodeHostsResourceModel struct {
	Node    types.String             `tfsdk:"node"`
	Entries []pveNodeHostsEntryModel `tfsdk:"entries"`
	Digest  types.String             `tfsdk:"digest"`
	ID      types.String             `tfsdk:"id"`
}

// pveNodeHostsEntryModel is one ordered /etc/hosts entry.
type pveNodeHostsEntryModel struct {
	Address   types.String `tfsdk:"address"`
	Hostnames types.List   `tfsdk:"hostnames"`
}

// Metadata implements resource.Resource.
func (r *pveNodeHostsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeHosts
}

// Schema implements resource.Resource.
func (r *pveNodeHostsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the `/etc/hosts` file of one node (`GET/POST /nodes/{node}/hosts`). Every write replaces the whole file, so the `entries` list is authoritative and other (out-of-band) lines are lost on apply. Removing this resource from configuration only forgets the state — the file on the node is left untouched because PVE offers no way to delete `/etc/hosts` content.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose `/etc/hosts` is managed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"entries": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "Ordered entries of the hosts file; each renders as `<address> <hostname>...` on its own line. The list order is the file order.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"address": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "IP address of the entry.",
						},
						"hostnames": schema.ListAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Hostnames of the entry; the first is the canonical name.",
						},
					},
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the current file as reported by the node; used for concurrent-modification detection on write.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the resource; equals `node`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeHostsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create implements resource.Resource: read the current digest, POST the
// whole file, then re-read because hostsd may normalize the content.
func (r *pveNodeHostsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeHostsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_node_hosts", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()

	current, err := r.client.GetNodeHosts(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_hosts", fmt.Sprintf("reading hosts (GET /nodes/%s/hosts) for node %s: %s", node, node, err))
		return
	}
	data := nodeHostsRender(plan.Entries)
	if err := r.client.SetNodeHosts(ctx, node, data, current.Digest); err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_hosts", fmt.Sprintf("writing hosts (POST /nodes/%s/hosts) for node %s: %s", node, node, err))
		return
	}
	if diags := r.readInto(ctx, &plan); diags != nil {
		resp.Diagnostics.AddError("Error creating pve_node_hosts", diags.message)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeHostsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeHostsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if diags := r.readInto(ctx, &state); diags != nil {
		if isPVEClientNotFound(diags.err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_node_hosts", diags.message)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Writes are whole-file, so update
// is the same POST as create.
func (r *pveNodeHostsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeHostsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_node_hosts", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()

	current, err := r.client.GetNodeHosts(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error updating pve_node_hosts", fmt.Sprintf("reading hosts (GET /nodes/%s/hosts) for node %s: %s", node, node, err))
		return
	}
	data := nodeHostsRender(plan.Entries)
	if err := r.client.SetNodeHosts(ctx, node, data, current.Digest); err != nil {
		resp.Diagnostics.AddError("Error updating pve_node_hosts", fmt.Sprintf("writing hosts (POST /nodes/%s/hosts) for node %s: %s", node, node, err))
		return
	}
	if diags := r.readInto(ctx, &plan); diags != nil {
		if isPVEClientNotFound(diags.err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error updating pve_node_hosts", diags.message)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. /etc/hosts cannot be emptied
// through the API, so delete only forgets the state.
func (r *pveNodeHostsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState implements resource.ResourceWithImportState. The import ID
// is the node name.
func (r *pveNodeHostsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("node"), req, resp)
}

// nodeHostsReadError bundles a read failure for the 404-vs-diagnostic
// decision of the callers.
type nodeHostsReadError struct {
	err     error
	message string
}

// readInto refreshes the model from the node: GET the file, parse it,
// and update entries, digest, and id. A nil return means success.
func (r *pveNodeHostsResource) readInto(ctx context.Context, model *pveNodeHostsResourceModel) *nodeHostsReadError {
	node := model.Node.ValueString()
	hosts, err := r.client.GetNodeHosts(ctx, node)
	if err != nil {
		return &nodeHostsReadError{err: err, message: fmt.Sprintf("reading hosts (GET /nodes/%s/hosts) for node %s: %s", node, node, err)}
	}
	model.Entries = nodeHostsParse(hosts.Data)
	model.Digest = nodeNetworkStringToTF(hosts.Digest)
	model.ID = types.StringValue(node)
	return nil
}

// nodeHostsRender renders the entries to the /etc/hosts file format:
// one `<address> <hostname>...` line per entry.
func nodeHostsRender(entries []pveNodeHostsEntryModel) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		fields := append([]string{entry.Address.ValueString()}, listStringFromTF(entry.Hostnames)...)
		lines = append(lines, strings.Join(fields, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

// nodeHostsParse parses /etc/hosts content into entries, skipping blank
// lines and `#` comments.
func nodeHostsParse(data string) []pveNodeHostsEntryModel {
	entries := make([]pveNodeHostsEntryModel, 0)
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		entries = append(entries, pveNodeHostsEntryModel{
			Address:   types.StringValue(fields[0]),
			Hostnames: listStringToTF(fields[1:]),
		})
	}
	return entries
}
