// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	aptEnterpriseBase = "https://enterprise.proxmox.com/debian"
	aptDownloadBase   = "http://download.proxmox.com/debian"
)

// aptHandleRe is upstream's standard repository handle grammar.
var aptHandleRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// aptChannels are checked in this order when parsing a handle.
var aptChannels = []string{"no-subscription", "enterprise", "test"}

// aptNodeLocks serializes mutations per node: the PVE apt endpoints
// read-modify-write the same .sources files.
var aptNodeLocks sync.Map // node -> *sync.Mutex

func lockAptNode(node string) func() {
	m, _ := aptNodeLocks.LoadOrStore(node, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// aptStandardRepositoryResource enables or disables one standard APT
// repository of a node.
type aptStandardRepositoryResource struct {
	client *pveclient.Client
}

var (
	_ resource.Resource                = (*aptStandardRepositoryResource)(nil)
	_ resource.ResourceWithConfigure   = (*aptStandardRepositoryResource)(nil)
	_ resource.ResourceWithImportState = (*aptStandardRepositoryResource)(nil)
)

// NewAptStandardRepositoryResource is the resource constructor.
func NewAptStandardRepositoryResource() resource.Resource {
	return &aptStandardRepositoryResource{}
}

type aptStandardRepositoryModel struct {
	ID      types.String `tfsdk:"id"`
	Node    types.String `tfsdk:"node"`
	Handle  types.String `tfsdk:"handle"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Name    types.String `tfsdk:"name"`
}

// pveBool decodes PVE booleans, which may be JSON booleans, 0/1 numbers or
// strings.
type pveBool bool

func (b *pveBool) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		*b = pveBool(x)
	case float64:
		*b = pveBool(x != 0)
	case string:
		p, err := strconv.ParseBool(strings.TrimSpace(x))
		if err != nil {
			return fmt.Errorf("invalid boolean %q", x)
		}
		*b = pveBool(p)
	default:
		return fmt.Errorf("invalid boolean %s", data)
	}
	return nil
}

type aptRepositoriesResponse struct {
	Files         []aptRepositoryFile `json:"files"`
	StandardRepos []aptStandardRepo   `json:"standard-repos"`
}

type aptRepositoryFile struct {
	Path         string               `json:"path"`
	Repositories []aptRepositoryEntry `json:"repositories"`
}

type aptRepositoryEntry struct {
	Types      []string `json:"Types"`
	URIs       []string `json:"URIs"`
	Suites     []string `json:"Suites"`
	Components []string `json:"Components"`
	Enabled    pveBool  `json:"Enabled"`
	FileType   string   `json:"FileType"`
	Comment    string   `json:"Comment"`
}

type aptStandardRepo struct {
	Handle string   `json:"handle"`
	Name   string   `json:"name"`
	Status *pveBool `json:"status"`
}

func aptRepositoriesPath(node string) string {
	return "/nodes/" + url.PathEscape(node) + "/apt/repositories"
}

func readAptRepositories(ctx context.Context, c *pveclient.Client, node string) (aptRepositoriesResponse, error) {
	p := aptRepositoriesPath(node)
	var resp aptRepositoriesResponse
	payload, err := c.DoRequest(ctx, "GET", p, nil)
	if err != nil {
		return resp, fmt.Errorf("GET %s: %w", p, err)
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return resp, fmt.Errorf("GET %s: decoding response: %w", p, err)
	}
	return resp, nil
}

func (r aptRepositoriesResponse) standard(handle string) (aptStandardRepo, bool) {
	for _, s := range r.StandardRepos {
		if s.Handle == handle {
			return s, true
		}
	}
	return aptStandardRepo{}, false
}

func (s aptStandardRepo) enabled() bool { return s.Status != nil && bool(*s.Status) }

func aptBase(channel string) string {
	if channel == "enterprise" {
		return aptEnterpriseBase
	}
	return aptDownloadBase
}

// aptHandleMatch returns the URIs and components identifying the apt entries
// of a standard repository handle (product pve).
func aptHandleMatch(handle string) (uris, components []string, ok bool) {
	for _, ch := range aptChannels {
		if handle == ch {
			base := aptBase(ch)
			components = []string{"pve-" + ch}
			if ch == "test" {
				components = append(components, "pvetest")
			}
			return []string{base + "/pve", base}, components, true
		}
	}
	rest, found := strings.CutPrefix(handle, "ceph-")
	if !found {
		return nil, nil, false
	}
	for _, ch := range aptChannels {
		release, found := strings.CutSuffix(rest, "-"+ch)
		if found && release != "" {
			return []string{aptBase(ch) + "/ceph-" + release}, []string{ch}, true
		}
	}
	return nil, nil, false
}

// aptEntryMatches mirrors upstream's handle match without the suite check, so
// disabling also turns off entries with stale suites.
func aptEntryMatches(e aptRepositoryEntry, uris, components []string) bool {
	hasDeb := false
	for _, t := range e.Types {
		if t == "deb" {
			hasDeb = true
			break
		}
	}
	if !hasDeb {
		return false
	}
	uriOK := false
	for _, u := range e.URIs {
		u = strings.TrimRight(u, "/")
		for _, w := range uris {
			if u == w {
				uriOK = true
			}
		}
	}
	if !uriOK {
		return false
	}
	for _, c := range e.Components {
		for _, w := range components {
			if c == w {
				return true
			}
		}
	}
	return false
}

func (r *aptStandardRepositoryResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "pve_apt_standard_repository"
}

func (r *aptStandardRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables or disables one standard APT repository (`enterprise`, `no-subscription`, `test`, `ceph-squid-no-subscription`, …) on a node. Enabling calls `PUT /nodes/{node}/apt/repositories` (`add_repository`), which enables an existing disabled entry or writes a new one. Disabling sets `enabled=0` on every enabled `/etc/apt` entry that matches the repository's URI and component. Use `for_each` to manage several repositories.\n\n" +
			"Setting `enabled = false` on a repository that is not configured does nothing. Destroying the resource disables the repository. Entries pointing at a local mirror cannot be matched, so the provider cannot disable them.\n\n" +
			"Import with `<node>/<handle>`, for example `pve1/no-subscription`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`<node>/<handle>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Node whose APT configuration is managed.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(nodeNameRe, "must be a valid node name")},
			},
			"handle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Standard repository handle, for example `enterprise`, `no-subscription`, `test` or `ceph-squid-no-subscription`. The node's `standard_repos` (see the `pve_apt_repositories` data source) lists the offered handles.",
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.RegexMatches(aptHandleRe, "must be a standard repository handle such as no-subscription or ceph-squid-enterprise")},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the repository is enabled. Defaults to `true`.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full repository name reported by the node.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *aptStandardRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", fmt.Sprintf("Expected *pveclient.Client for pve_apt_standard_repository, got: %T.", req.ProviderData))
		return
	}
	r.client = c
}

func statusText(s aptStandardRepo, found bool) string {
	switch {
	case !found || s.Status == nil:
		return "not configured"
	case bool(*s.Status):
		return "enabled"
	default:
		return "disabled"
	}
}

// reconcile brings the repository to the wanted state. ok=false means the
// node does not offer handle; offered then lists every handle it does offer.
func (r *aptStandardRepositoryResource) reconcile(ctx context.Context, node, handle string, want bool) (name string, offered []string, ok bool, diags diag.Diagnostics) {
	if r.client == nil {
		diags.AddError("Client Not Configured", "pve_apt_standard_repository: provider client is not configured.")
		return
	}
	unlock := lockAptNode(node)
	defer unlock()

	p := aptRepositoriesPath(node)
	resp, err := readAptRepositories(ctx, r.client, node)
	if err != nil {
		diags.AddError("Error Reading APT Repositories", err.Error())
		return
	}
	std, found := resp.standard(handle)
	if !found {
		for _, s := range resp.StandardRepos {
			offered = append(offered, s.Handle)
		}
		return "", offered, false, nil
	}
	enabledNow := std.enabled()
	changed := false

	if want && !enabledNow {
		if _, err := r.client.DoRequest(ctx, "PUT", p, url.Values{"handle": {handle}}); err != nil {
			diags.AddError("Error Enabling Standard Repository", fmt.Sprintf("PUT %s handle=%s: %s", p, handle, err))
			return
		}
		changed = true
	}
	if !want && enabledNow {
		uris, components, known := aptHandleMatch(handle)
		if !known {
			diags.AddError("Unsupported Standard Repository", fmt.Sprintf("pve_apt_standard_repository does not know which apt entries %q maps to, so it cannot disable it. Disable it on node %s under Updates → Repositories.", handle, node))
			return
		}
		type target struct {
			path  string
			index int
		}
		var targets []target
		for _, f := range resp.Files {
			for i, e := range f.Repositories {
				if bool(e.Enabled) && aptEntryMatches(e, uris, components) {
					targets = append(targets, target{f.Path, i})
				}
			}
		}
		if len(targets) == 0 {
			diags.AddError("Repository Entry Not Found", fmt.Sprintf("Node %s reports standard repository %q as enabled, but no entry in its apt sources matches %s with component %s (mirrors are not matched). Disable it on the node under Updates → Repositories.", node, handle, strings.Join(uris, " or "), strings.Join(components, " or ")))
			return
		}
		for _, t := range targets {
			params := url.Values{"path": {t.path}, "index": {strconv.Itoa(t.index)}, "enabled": {"0"}}
			if _, err := r.client.DoRequest(ctx, "POST", p, params); err != nil {
				diags.AddError("Error Disabling Standard Repository", fmt.Sprintf("POST %s path=%s index=%d: %s", p, t.path, t.index, err))
				return
			}
		}
		changed = true
	}

	if changed {
		resp, err = readAptRepositories(ctx, r.client, node)
		if err != nil {
			diags.AddError("Error Reading APT Repositories", err.Error())
			return
		}
		std, found = resp.standard(handle)
		if found && std.enabled() != want || !found {
			wantText := "disabled"
			if want {
				wantText = "enabled"
			}
			diags.AddError("Standard Repository State Mismatch", fmt.Sprintf("After the change, node %s reports standard repository %q as %s; wanted %s.", node, handle, statusText(std, found), wantText))
			return
		}
	}
	return std.Name, nil, true, diags
}

func (r *aptStandardRepositoryResource) apply(ctx context.Context, m *aptStandardRepositoryModel) diag.Diagnostics {
	node, handle := m.Node.ValueString(), m.Handle.ValueString()
	name, offered, ok, diags := r.reconcile(ctx, node, handle, m.Enabled.ValueBool())
	if diags.HasError() {
		return diags
	}
	if !ok {
		diags.AddError("Unknown Standard Repository", fmt.Sprintf("Node %s does not offer standard repository %q. Offered handles: %s.", node, handle, strings.Join(offered, ", ")))
		return diags
	}
	m.ID = types.StringValue(node + "/" + handle)
	m.Name = types.StringValue(name)
	return diags
}

func (r *aptStandardRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m aptStandardRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update only sees `enabled` changes: node and handle force replacement.
func (r *aptStandardRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m aptStandardRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *aptStandardRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m aptStandardRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "pve_apt_standard_repository: provider client is not configured.")
		return
	}
	node, handle := m.Node.ValueString(), m.Handle.ValueString()
	apt, err := readAptRepositories(ctx, r.client, node)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading APT Repositories", err.Error())
		return
	}
	std, found := apt.standard(handle)
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	m.Enabled = types.BoolValue(std.enabled())
	m.Name = types.StringValue(std.Name)
	m.ID = types.StringValue(node + "/" + handle)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Delete disables the repository.
func (r *aptStandardRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m aptStandardRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// !ok: the handle is no longer offered, so there is nothing to disable.
	_, _, _, diags := r.reconcile(ctx, m.Node.ValueString(), m.Handle.ValueString(), false)
	resp.Diagnostics.Append(diags...)
}

// ImportState takes `<node>/<handle>`; Read fills in the rest.
func (r *aptStandardRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, handle, ok := strings.Cut(req.ID, "/")
	if !ok || !nodeNameRe.MatchString(node) || !aptHandleRe.MatchString(handle) {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected <node>/<handle> (for example pve1/no-subscription), got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("handle"), handle)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), node+"/"+handle)...)
}
