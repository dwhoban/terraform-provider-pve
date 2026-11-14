// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var aptFilePathRe = regexp.MustCompile(`^/\S+$`)

// aptRepositoryResource toggles one existing entry of a node's APT sources.
// The PVE API cannot create or delete entries.
type aptRepositoryResource struct {
	client *pveclient.Client
}

var (
	_ resource.Resource                = (*aptRepositoryResource)(nil)
	_ resource.ResourceWithConfigure   = (*aptRepositoryResource)(nil)
	_ resource.ResourceWithImportState = (*aptRepositoryResource)(nil)
)

// NewAptRepositoryResource is the resource constructor.
func NewAptRepositoryResource() resource.Resource { return &aptRepositoryResource{} }

type aptRepositoryModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	FilePath     types.String `tfsdk:"file_path"`
	Index        types.Int64  `tfsdk:"index"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	URIs         types.List   `tfsdk:"uris"`
	Suites       types.List   `tfsdk:"suites"`
	Components   types.List   `tfsdk:"components"`
	PackageTypes types.List   `tfsdk:"package_types"`
	FileType     types.String `tfsdk:"file_type"`
	Comment      types.String `tfsdk:"comment"`
}

func findAptEntry(r aptRepositoriesResponse, filePath string, index int64) (aptRepositoryEntry, bool) {
	if index < 0 {
		return aptRepositoryEntry{}, false
	}
	for _, f := range r.Files {
		if f.Path == filePath && index < int64(len(f.Repositories)) {
			return f.Repositories[index], true
		}
	}
	return aptRepositoryEntry{}, false
}

// parseAptRepositoryImportID splits `<node>,<file_path>,<index>`; the path may
// itself contain commas, so the index is taken after the last comma.
func parseAptRepositoryImportID(id string) (node, filePath string, index int64, ok bool) {
	node, rest, found := strings.Cut(id, ",")
	if !found || !nodeNameRe.MatchString(node) {
		return "", "", 0, false
	}
	i := strings.LastIndex(rest, ",")
	if i < 0 {
		return "", "", 0, false
	}
	filePath = rest[:i]
	n, err := strconv.ParseInt(rest[i+1:], 10, 64)
	if err != nil || n < 0 || !strings.HasPrefix(filePath, "/") {
		return "", "", 0, false
	}
	return node, filePath, n, true
}

func strList(ctx context.Context, s []string) (types.List, diag.Diagnostics) {
	if s == nil {
		s = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, s)
}

func (m *aptRepositoryModel) apply(ctx context.Context, e aptRepositoryEntry) diag.Diagnostics {
	var diags, d diag.Diagnostics
	m.Enabled = types.BoolValue(bool(e.Enabled))
	m.FileType = types.StringValue(e.FileType)
	m.Comment = types.StringValue(e.Comment)
	m.URIs, d = strList(ctx, e.URIs)
	diags.Append(d...)
	m.Suites, d = strList(ctx, e.Suites)
	diags.Append(d...)
	m.Components, d = strList(ctx, e.Components)
	diags.Append(d...)
	m.PackageTypes, d = strList(ctx, e.Types)
	diags.Append(d...)
	m.ID = types.StringValue(fmt.Sprintf("%s,%s,%d", m.Node.ValueString(), m.FilePath.ValueString(), m.Index.ValueInt64()))
	return diags
}

func (r *aptRepositoryResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "pve_apt_repository"
}

func (r *aptRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	list := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables or disables one **existing** entry of a node's APT sources (`/etc/apt/sources.list` or `sources.list.d`), including non-standard repositories. Find `file_path` and `index` with the `pve_apt_repositories` data source.\n\n" +
			"The resource only toggles `enabled` through `POST /nodes/{node}/apt/repositories`. The Proxmox VE API cannot create or delete entries, so the entry must already exist; use `pve_apt_standard_repository` for standard repositories. Destroying the resource only removes it from state and leaves the entry unchanged. Entry indexes shift when a file is edited, so the entry's `uris`, `suites` and `components` are read back on every refresh.\n\n" +
			"Import with `<node>,<file_path>,<index>`, for example `pve1,/etc/apt/sources.list.d/ceph.sources,0`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`<node>,<file_path>,<index>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Node whose APT configuration is managed.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(nodeNameRe, "must be a valid node name")},
			},
			"file_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Absolute path of the sources file that contains the entry.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(aptFilePathRe, "must be an absolute path")},
			},
			"index": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Index of the entry within the file, starting at 0.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the entry is enabled. When omitted, the entry's current state is adopted and left unchanged.",
			},
			"uris":          list("Repository URIs."),
			"suites":        list("Suites (distributions)."),
			"components":    list("Components."),
			"package_types": list("Package types, for example `deb`."),
			"file_type":     schema.StringAttribute{Computed: true, MarkdownDescription: "Format of the sources file (`list` or `sources`)."},
			"comment":       schema.StringAttribute{Computed: true, MarkdownDescription: "Comment attached to the entry."},
		},
	}
}

func (r *aptRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", fmt.Sprintf("Expected *pveclient.Client for pve_apt_repository, got: %T.", req.ProviderData))
		return
	}
	r.client = c
}

func enabledText(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

// load reads the node's repositories and locates the entry.
func (r *aptRepositoryResource) load(ctx context.Context, node, filePath string, index int64) (aptRepositoryEntry, diag.Diagnostics) {
	var diags diag.Diagnostics
	resp, err := readAptRepositories(ctx, r.client, node)
	if err != nil {
		diags.AddError("Error Reading APT Repositories", err.Error())
		return aptRepositoryEntry{}, diags
	}
	e, ok := findAptEntry(resp, filePath, index)
	if !ok {
		diags.AddError("Repository Entry Not Found", fmt.Sprintf("Node %s has no APT entry at index %d of %s. The PVE API cannot create entries; check the pve_apt_repositories data source for existing ones.", node, index, filePath))
	}
	return e, diags
}

// converge makes the entry match m.Enabled (when set) and fills m from the
// final live entry.
func (r *aptRepositoryResource) converge(ctx context.Context, m *aptRepositoryModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.client == nil {
		diags.AddError("Client Not Configured", "pve_apt_repository: provider client is not configured.")
		return diags
	}
	node, filePath, index := m.Node.ValueString(), m.FilePath.ValueString(), m.Index.ValueInt64()
	unlock := lockAptNode(node)
	defer unlock()

	e, d := r.load(ctx, node, filePath, index)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() && m.Enabled.ValueBool() != bool(e.Enabled) {
		want := m.Enabled.ValueBool()
		p := aptRepositoriesPath(node)
		params := url.Values{"path": {filePath}, "index": {strconv.FormatInt(index, 10)}, "enabled": {strconv.Itoa(b2iProd(want))}}
		if _, err := r.client.DoRequest(ctx, "POST", p, params); err != nil {
			diags.AddError("Error Changing APT Repository", fmt.Sprintf("POST %s path=%s index=%d: %s", p, filePath, index, err))
			return diags
		}
		e, d = r.load(ctx, node, filePath, index)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		if bool(e.Enabled) != want {
			diags.AddError("APT Repository State Mismatch", fmt.Sprintf("After the change, entry %d of %s on node %s is %s; wanted %s.", index, filePath, node, enabledText(bool(e.Enabled)), enabledText(want)))
			return diags
		}
	}
	diags.Append(m.apply(ctx, e)...)
	return diags
}

func b2iProd(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *aptRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m aptRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.converge(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update only sees `enabled` changes: every other input forces replacement.
func (r *aptRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m aptRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.converge(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *aptRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m aptRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "pve_apt_repository: provider client is not configured.")
		return
	}
	apt, err := readAptRepositories(ctx, r.client, m.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading APT Repositories", err.Error())
		return
	}
	e, ok := findAptEntry(apt, m.FilePath.ValueString(), m.Index.ValueInt64())
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(m.apply(ctx, e)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *aptRepositoryResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// The PVE API cannot remove entries; the entry is left as is.
}

// ImportState takes `<node>,<file_path>,<index>`; Read fills in the rest.
func (r *aptRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, filePath, index, ok := parseAptRepositoryImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected <node>,<file_path>,<index> (for example pve1,/etc/apt/sources.list,0), got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("file_path"), filePath)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("index"), index)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
