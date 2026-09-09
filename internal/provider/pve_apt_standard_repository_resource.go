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
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveAptStandardRepositoryResource{}
	_ resource.ResourceWithImportState = &pveAptStandardRepositoryResource{}
)

// NewPveAptStandardRepositoryResource returns the resource implementation.
func NewPveAptStandardRepositoryResource() resource.Resource {
	return &pveAptStandardRepositoryResource{}
}

// pveAptStandardRepositoryResource manages the presence of one standard APT
// repository (identified by its PVE handle, e.g. `no-subscription` or
// `enterprise`) in a node's repository configuration. Create issues the
// pin's add form (PUT /nodes/{node}/apt/repositories). The pinned API has
// no remove verb (its change form only toggles enabled/disabled), so Delete
// forgets the resource from state without touching the node — documented in
// the schema description.
type pveAptStandardRepositoryResource struct {
	client *pveclient.Client
}

// pveAptStandardRepositoryResourceModel is the Terraform-facing shape.
type pveAptStandardRepositoryResourceModel struct {
	Node   types.String `tfsdk:"node"`
	Handle types.String `tfsdk:"handle"`
	Name   types.String `tfsdk:"name"`
	Status types.Bool   `tfsdk:"status"`
}

// Metadata implements resource.Resource.
func (r *pveAptStandardRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAptStandardRepository
}

// Schema implements resource.Resource.
func (r *pveAptStandardRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the presence of one standard APT repository on a node (`PUT /nodes/{node}/apt/repositories` with the repository handle, e.g. `enterprise`, `no-subscription`, or a `ceph-<release>` handle). Terraform adoption is idempotent: applying against an already-configured repository succeeds. Destroying the resource only removes it from state: the pinned API defines no way to remove a repository entry (its change form only toggles enabled/disabled), so the node configuration is left untouched. Use `data.pve_node_apt_repositories` to inspect the resulting files. Changing `node` or `handle` forces recreation.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose repository configuration gains the standard repository.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"handle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Handle that identifies the standard repository (pin `handle` parameter, e.g. `enterprise`, `no-subscription`, `ceph-quincy`). Unknown handles are rejected by the server. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full name of the repository as reported by PVE.",
			},
			"status": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the configured repository is enabled.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveAptStandardRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create implements resource.Resource.
func (r *pveAptStandardRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveAptStandardRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_apt_standard_repository", "provider client is not configured")
		return
	}
	node, handle := plan.Node.ValueString(), plan.Handle.ValueString()
	present, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_apt_standard_repository",
			fmt.Sprintf("reading APT repositories on node %s: %s", node, err),
		)
		return
	}
	if !present {
		tflog.Info(ctx, "adding standard apt repository", map[string]any{"node": node, "handle": handle})
		if err := r.client.AddAptStandardRepository(ctx, node, handle); err != nil {
			resp.Diagnostics.AddError(
				"Error creating pve_apt_standard_repository",
				fmt.Sprintf("adding standard repository %s on node %s: %s", handle, node, err),
			)
			return
		}
		present, err = r.readInto(ctx, &plan)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_apt_standard_repository after create",
				fmt.Sprintf("reading APT repositories on node %s: %s", node, err),
			)
			return
		}
		if !present {
			resp.Diagnostics.AddError(
				"Error creating pve_apt_standard_repository",
				fmt.Sprintf("standard repository %s on node %s is not configured after the add request", handle, node),
			)
			return
		}
	} else {
		tflog.Info(ctx, "standard apt repository already configured, adopting", map[string]any{"node": node, "handle": handle})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveAptStandardRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveAptStandardRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error reading pve_apt_standard_repository", "provider client is not configured")
		return
	}
	present, err := r.readInto(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_apt_standard_repository",
			fmt.Sprintf("reading APT repositories on node %s: %s", state.Node.ValueString(), err),
		)
		return
	}
	if !present {
		// The repository is no longer configured (removed out of band or
		// disabled-and-detached); drop it from state.
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Both configurable attributes force
// replacement, so updates only refresh the computed values; a repository
// that vanished out of band surfaces an error instead of a silent re-add.
func (r *pveAptStandardRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveAptStandardRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_apt_standard_repository", "provider client is not configured")
		return
	}
	present, err := r.readInto(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_apt_standard_repository",
			fmt.Sprintf("reading APT repositories on node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	if !present {
		resp.Diagnostics.AddError(
			"Error updating pve_apt_standard_repository",
			fmt.Sprintf("standard repository %s on node %s is no longer configured; recreate the resource", plan.Handle.ValueString(), plan.Node.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The pinned API defines no remove
// verb for repository entries (the change form only toggles enabled), so
// delete forgets the resource from state and leaves the node untouched.
func (r *pveAptStandardRepositoryResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState parses an import ID of the form `<node>:<handle>`.
func (r *pveAptStandardRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_apt_standard_repository import ID",
			fmt.Sprintf("import ID must be `<node>:<handle>`, e.g. `pve1:no-subscription`; got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("handle"), parts[1])...)
}

// readInto refreshes the model from the node's repository listing. It
// reports whether the standard repository is configured (present in the
// listing with a non-null status); computed fields are updated in place.
func (r *pveAptStandardRepositoryResource) readInto(ctx context.Context, m *pveAptStandardRepositoryResourceModel) (bool, error) {
	repos, err := r.client.GetNodeAptRepositories(ctx, m.Node.ValueString())
	if err != nil {
		return false, err
	}
	m.Name = types.StringNull()
	m.Status = types.BoolNull()
	for _, repo := range repos.StandardRepositories {
		if repo.Handle != m.Handle.ValueString() {
			continue
		}
		m.Name = types.StringValue(repo.Name)
		m.Status = nodeNetworkBoolPtrToTF(repo.Status)
		return repo.Status != nil, nil
	}
	return false, nil
}
