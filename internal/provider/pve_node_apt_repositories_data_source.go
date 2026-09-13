// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource = &pveNodeAptRepositoriesDataSource{}
)

// NewPveNodeAptRepositoriesDataSource returns the data source implementation.
func NewPveNodeAptRepositoriesDataSource() datasource.DataSource {
	return &pveNodeAptRepositoriesDataSource{}
}

// pveNodeAptRepositoriesDataSource exposes the parsed APT repository
// configuration of one node (GET /nodes/{node}/apt/repositories). The
// per-file repository entries from the pin are flattened into one
// `repositories` list carrying the containing file path and the index
// within that file, so the rows can be fed directly to the upstream
// repository-change endpoint.
type pveNodeAptRepositoriesDataSource struct {
	client *pveclient.Client
}

// pveNodeAptRepositoriesDataSourceModel is the Terraform-facing shape.
type pveNodeAptRepositoriesDataSourceModel struct {
	ID                   types.String                                   `tfsdk:"id"`
	Node                 types.String                                   `tfsdk:"node"`
	Package              types.String                                   `tfsdk:"package"`
	Changelog            types.String                                   `tfsdk:"changelog"`
	Digest               types.String                                   `tfsdk:"digest"`
	Repositories         []pveNodeAptRepositoriesDataSourceRepo         `tfsdk:"repositories"`
	StandardRepositories []pveNodeAptRepositoriesDataSourceStandardRepo `tfsdk:"standard_repositories"`
	Infos                []pveNodeAptRepositoriesDataSourceInfo         `tfsdk:"infos"`
	Errors               []pveNodeAptRepositoriesDataSourceError        `tfsdk:"errors"`
}

// pveNodeAptRepositoriesDataSourceRepo mirrors one parsed repository entry,
// flattened with its containing file path and position.
type pveNodeAptRepositoriesDataSourceRepo struct {
	Path       types.String                              `tfsdk:"path"`
	Index      types.Int64                               `tfsdk:"index"`
	Comment    types.String                              `tfsdk:"comment"`
	Types      types.List                                `tfsdk:"types"`
	URIs       types.List                                `tfsdk:"uris"`
	Suites     types.List                                `tfsdk:"suites"`
	Components types.List                                `tfsdk:"components"`
	Enabled    types.Bool                                `tfsdk:"enabled"`
	Options    []pveNodeAptRepositoriesDataSourceOptions `tfsdk:"options"`
}

// pveNodeAptRepositoriesDataSourceOptions mirrors one repository option row.
type pveNodeAptRepositoriesDataSourceOptions struct {
	Key    types.String `tfsdk:"key"`
	Values types.List   `tfsdk:"values"`
}

// pveNodeAptRepositoriesDataSourceStandardRepo mirrors one standard
// repository row. Status is null when the repository is not configured.
type pveNodeAptRepositoriesDataSourceStandardRepo struct {
	Handle types.String `tfsdk:"handle"`
	Name   types.String `tfsdk:"name"`
	Status types.Bool   `tfsdk:"status"`
}

// pveNodeAptRepositoriesDataSourceInfo mirrors one info/warning row.
type pveNodeAptRepositoriesDataSourceInfo struct {
	Index    types.String `tfsdk:"index"`
	Kind     types.String `tfsdk:"kind"`
	Message  types.String `tfsdk:"message"`
	Path     types.String `tfsdk:"path"`
	Property types.String `tfsdk:"property"`
}

// pveNodeAptRepositoriesDataSourceError mirrors one problematic-file row.
type pveNodeAptRepositoriesDataSourceError struct {
	Path  types.String `tfsdk:"path"`
	Error types.String `tfsdk:"error"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeAptRepositoriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeAptRepositories
}

// Schema implements datasource.DataSource.
func (d *pveNodeAptRepositoriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the parsed APT repository configuration of one node (`GET /nodes/{node}/apt/repositories`): every repository entry from the parsed sources files, the standard repositories and their configuration status, and any parsing warnings or errors. `repositories` rows are flattened with their containing `path` and zero-based `index` within that file, matching the coordinates the upstream repository-change endpoint expects.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source (the node name).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose APT repositories to read.",
			},
			"package": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Package name to additionally fetch the changelog for (`GET /nodes/{node}/apt/changelog`). When set, `changelog` carries the package's raw changelog text; when unset, `changelog` is null.",
			},
			"changelog": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw changelog text of `package` as reported by the node. Null when `package` is not set.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Common digest of all parsed repository files; usable as an optimistic-concurrency guard on repository changes.",
			},
			"repositories": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every parsed repository entry, flattened across files in file order.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Path of the file containing the repository entry.",
						},
						"index": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Zero-based position of the entry within its file.",
						},
						"comment": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Associated comment.",
						},
						"types": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Package types of the entry. Values: `deb`, `deb-src`.",
						},
						"uris": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Repository URIs.",
						},
						"suites": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Package distributions (suites).",
						},
						"components": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Repository components. Empty for `.sources` entries without components.",
						},
						"enabled": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the repository is enabled.",
						},
						"options": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Additional options of the entry.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"key": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Option key (e.g. `Signed-By`).",
									},
									"values": schema.ListAttribute{
										Computed:            true,
										ElementType:         types.StringType,
										MarkdownDescription: "Option values.",
									},
								},
							},
						},
					},
				},
			},
			"standard_repositories": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All standard repositories known to PVE and their configuration status.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"handle": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Handle identifying the repository (e.g. `enterprise`, `no-subscription`).",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Full name of the repository.",
						},
						"status": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the configured repository is enabled. Null when the repository is not configured.",
						},
					},
				},
			},
			"infos": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Additional information and warnings for the APT repositories.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"index": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Index of the associated repository within the file, as a string.",
						},
						"kind": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Kind of the information (e.g. `warning`).",
						},
						"message": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Information message.",
						},
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Path of the associated file.",
						},
						"property": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Property from which the info originates.",
						},
					},
				},
			},
			"errors": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Repository files that could not be parsed.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Path of the problematic file.",
						},
						"error": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The error message.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeAptRepositoriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = nodeSvcAptConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNodeAptRepositoriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeAptRepositoriesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node_apt_repositories", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	repos, err := d.client.GetNodeAptRepositories(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_apt_repositories",
			fmt.Sprintf("reading APT repositories on node %s: %s", node, err),
		)
		return
	}
	data.ID = types.StringValue(node)
	data.Digest = types.StringValue(repos.Digest)
	data.Repositories = aptRepositoriesToTF(repos.Files)
	data.StandardRepositories = aptStandardRepositoriesToTF(repos.StandardRepositories)
	data.Infos = aptRepositoryInfosToTF(repos.Infos)
	data.Errors = aptRepositoryErrorsToTF(repos.Errors)
	if !data.Package.IsNull() {
		changelog, err := d.client.GetNodeAptChangelog(ctx, node, data.Package.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_node_apt_repositories",
				fmt.Sprintf("reading APT changelog for package %s on node %s: %s", data.Package.ValueString(), node, err),
			)
			return
		}
		data.Changelog = types.StringValue(changelog)
	} else {
		data.Changelog = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// aptRepositoriesToTF flattens the per-file repository entries into row
// models carrying the containing file path and position.
func aptRepositoriesToTF(files []pveclient.NodeAptRepositoryFile) []pveNodeAptRepositoriesDataSourceRepo {
	repoCount := 0
	for _, file := range files {
		repoCount += len(file.Repositories)
	}
	rows := make([]pveNodeAptRepositoriesDataSourceRepo, 0, repoCount)
	for _, file := range files {
		for idx, repo := range file.Repositories {
			options := make([]pveNodeAptRepositoriesDataSourceOptions, 0, len(repo.Options))
			for _, opt := range repo.Options {
				options = append(options, pveNodeAptRepositoriesDataSourceOptions{
					Key:    types.StringValue(opt.Key),
					Values: listStringToTF(opt.Values),
				})
			}
			rows = append(rows, pveNodeAptRepositoriesDataSourceRepo{
				Path:       types.StringValue(file.Path),
				Index:      types.Int64Value(int64(idx)),
				Comment:    types.StringValue(repo.Comment),
				Types:      listStringToTF(repo.Types),
				URIs:       listStringToTF(repo.URIs),
				Suites:     listStringToTF(repo.Suites),
				Components: listStringToTF(repo.Components),
				Enabled:    types.BoolValue(repo.Enabled),
				Options:    options,
			})
		}
	}
	return rows
}

// aptStandardRepositoriesToTF projects standard repository rows; status
// stays null for unconfigured repositories.
func aptStandardRepositoriesToTF(repos []pveclient.NodeAptStandardRepository) []pveNodeAptRepositoriesDataSourceStandardRepo {
	rows := make([]pveNodeAptRepositoriesDataSourceStandardRepo, 0, len(repos))
	for _, repo := range repos {
		rows = append(rows, pveNodeAptRepositoriesDataSourceStandardRepo{
			Handle: types.StringValue(repo.Handle),
			Name:   types.StringValue(repo.Name),
			Status: nodeNetworkBoolPtrToTF(repo.Status),
		})
	}
	return rows
}

// aptRepositoryInfosToTF projects info/warning rows.
func aptRepositoryInfosToTF(infos []pveclient.NodeAptRepositoryInfo) []pveNodeAptRepositoriesDataSourceInfo {
	rows := make([]pveNodeAptRepositoriesDataSourceInfo, 0, len(infos))
	for _, info := range infos {
		rows = append(rows, pveNodeAptRepositoriesDataSourceInfo{
			Index:    types.StringValue(info.Index),
			Kind:     types.StringValue(info.Kind),
			Message:  types.StringValue(info.Message),
			Path:     types.StringValue(info.Path),
			Property: types.StringValue(info.Property),
		})
	}
	return rows
}

// aptRepositoryErrorsToTF projects problematic-file rows.
func aptRepositoryErrorsToTF(errs []pveclient.NodeAptRepositoryError) []pveNodeAptRepositoriesDataSourceError {
	rows := make([]pveNodeAptRepositoriesDataSourceError, 0, len(errs))
	for _, e := range errs {
		rows = append(rows, pveNodeAptRepositoriesDataSourceError{
			Path:  types.StringValue(e.Path),
			Error: types.StringValue(e.Error),
		})
	}
	return rows
}
