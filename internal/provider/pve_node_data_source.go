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
	_ datasource.DataSource              = &pveNodeDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeDataSource{}
)

// NewPveNodeDataSource returns the node facts data source implementation.
func NewPveNodeDataSource() datasource.DataSource {
	return &pveNodeDataSource{}
}

// pveNodeDataSource exposes node-scoped facts that have no dedicated
// entity data source: certificates (GET /nodes/{node}/certificates/info),
// effective vzdump defaults (GET /nodes/{node}/vzdump/defaults), the
// PVE version (GET /nodes/{node}/version), and uptime (GET
// /nodes/{node}/status).
type pveNodeDataSource struct {
	client *pveclient.Client
}

// pveNodeDataSourceCertificateModel mirrors one entry of the
// certificates list.
type pveNodeDataSourceCertificateModel struct {
	Filename      types.String `tfsdk:"filename"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	Subject       types.String `tfsdk:"subject"`
	PEM           types.String `tfsdk:"pem"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
	NotBefore     types.Int64  `tfsdk:"not_before"`
	NotAfter      types.Int64  `tfsdk:"not_after"`
	SAN           types.List   `tfsdk:"san"`
}

// pveNodeDataSourceVersionModel mirrors GET /nodes/{node}/version.
type pveNodeDataSourceVersionModel struct {
	Release types.String `tfsdk:"release"`
	RepoID  types.String `tfsdk:"repoid"`
	Version types.String `tfsdk:"version"`
}

// pveNodeDataSourceModel is the Terraform-facing shape.
type pveNodeDataSourceModel struct {
	ID             types.String                                `tfsdk:"id"`
	Node           types.String                                `tfsdk:"node"`
	Uptime         types.Int64                                 `tfsdk:"uptime"`
	Certificates   []pveNodeDataSourceCertificateModel         `tfsdk:"certificates"`
	VzdumpDefaults *pveBackupJobsDataSourceVzdumpDefaultsModel `tfsdk:"vzdump_defaults"`
	PVEVersion     *pveNodeDataSourceVersionModel              `tfsdk:"pve_version"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNode
}

// pveNodeVzdumpDefaultsAttributes returns the attribute map of the
// vzdump defaults object, shared with the pve_backup_jobs data source.
func pveNodeVzdumpDefaultsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"all": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Backup all known guest systems on this host.",
		},
		"bwlimit": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "I/O bandwidth limit in KiB/s.",
		},
		"compress": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Dump file compression. One of `0`, `1`, `gzip`, `lzo`, `zstd`.",
		},
		"dumpdir": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Directory the resulting files are written to.",
		},
		"exclude": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Comma-separated list of excluded guest IDs.",
		},
		"exclude_path": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Shell globs excluded from container backups.",
		},
		"fleecing": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Backup fleecing options as a PVE property string (e.g. `enabled=0,storage=local`).",
		},
		"ionice": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "IO priority when using the BFQ scheduler; between 0 and 8 (8 is idle priority).",
		},
		"lockwait": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal time to wait for the global lock in minutes.",
		},
		"mailnotification": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Deprecated by PVE: when a notification mail is sent. One of `always`, `failure`.",
		},
		"mailto": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Deprecated by PVE: comma-separated list of email addresses or users receiving notifications.",
		},
		"mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Backup mode. One of `snapshot`, `suspend`, `stop`.",
		},
		"node": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Node the defaults are scoped to, when configured.",
		},
		"notes_template": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Template string for generating notes for the backup(s).",
		},
		"notification_mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Which notification system to use. One of `auto`, `legacy-sendmail`, `notification-system`.",
		},
		"pbs_change_detection_mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "PBS change detection mode for container backups. One of `legacy`, `data`, `metadata`.",
		},
		"performance": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Performance-related settings as a PVE property string (e.g. `max-workers=16,pbs-entries-max=1048576`).",
		},
		"pigz": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "pigz thread count for gzip compression.",
		},
		"pool": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Pool whose guest systems are backed up.",
		},
		"protected": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether backup(s) are marked as protected.",
		},
		"prune_backups": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Retention options as a PVE property string (e.g. `keep-all=1,keep-last=3`).",
		},
		"quiet": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether log output is suppressed on the node.",
		},
		"remove": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether older backups are pruned according to `prune_backups`.",
		},
		"script": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Hook script used by vzdump.",
		},
		"stdexcludes": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether temporary files and logs are excluded.",
		},
		"stop": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether running backup jobs on this host are stopped.",
		},
		"stopwait": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal time to wait until a guest system is stopped in minutes.",
		},
		"storage": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Storage the resulting file is written to.",
		},
		"tmpdir": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Directory for temporary files.",
		},
		"vmid": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Comma-separated list of guest IDs to back up.",
		},
		"zstd": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Zstd thread count (`0` means half of the available cores).",
		},
	}
}

// Schema implements datasource.DataSource.
func (d *pveNodeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Node facts for one cluster node. Reads four endpoints per refresh: `GET /nodes/{node}/status` (uptime), `GET /nodes/{node}/certificates/info` (certificates), `GET /nodes/{node}/vzdump/defaults` (vzdump_defaults), and `GET /nodes/{node}/version` (pve_version). Full runtime status (CPU, memory, root filesystem) is served by the separate `pve_node_status` data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source; equals `node`.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name.",
			},
			"uptime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Node uptime in seconds, from `GET /nodes/{node}/status`.",
			},
			"certificates": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Certificates installed on the node, in the order reported by `GET /nodes/{node}/certificates/info`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"filename":        schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate file name on the node (e.g. `pveproxy-ssl.pem`)."},
						"fingerprint":     schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate SHA 256 fingerprint."},
						"issuer":          schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate issuer name."},
						"subject":         schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate subject name."},
						"pem":             schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate in PEM format."},
						"public_key_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate's public key algorithm (e.g. `rsa`)."},
						"public_key_bits": schema.Int64Attribute{Computed: true, MarkdownDescription: "Certificate's public key size in bits."},
						"not_before":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Certificate's notBefore timestamp (UNIX epoch)."},
						"not_after":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Certificate's notAfter timestamp (UNIX epoch)."},
						"san": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Certificate's SubjectAlternativeName entries.",
						},
					},
				},
			},
			"vzdump_defaults": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Effective vzdump defaults of the node (`GET /nodes/{node}/vzdump/defaults`). The `fleecing`, `performance`, and `prune_backups` fields arrive as PVE property strings on this endpoint and are surfaced verbatim.",
				Attributes:          pveNodeVzdumpDefaultsAttributes(),
			},
			"pve_version": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "PVE version details of the node (`GET /nodes/{node}/version`).",
				Attributes: map[string]schema.Attribute{
					"release": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The current installed Proxmox VE release (e.g. `8.4`).",
					},
					"repoid": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The short git commit hash ID from which this version was built.",
					},
					"version": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The current installed pve-manager package version.",
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource. Each endpoint read names itself
// in the diagnostic on failure.
func (d *pveNodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()

	status, err := d.client.GetNodeStatus(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node", fmt.Sprintf("reading status (GET /nodes/%s/status) for node %s: %s", node, node, err))
		return
	}
	data.Uptime = types.Int64Value(status.Uptime)

	certs, err := d.client.GetNodeCertificates(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node", fmt.Sprintf("reading certificates (GET /nodes/%s/certificates/info) for node %s: %s", node, node, err))
		return
	}
	data.Certificates = make([]pveNodeDataSourceCertificateModel, 0, len(certs))
	for _, cert := range certs {
		data.Certificates = append(data.Certificates, pveNodeDataSourceCertificateModel{
			Filename:      nodeNetworkStringToTF(cert.Filename),
			Fingerprint:   nodeNetworkStringToTF(cert.Fingerprint),
			Issuer:        nodeNetworkStringToTF(cert.Issuer),
			Subject:       nodeNetworkStringToTF(cert.Subject),
			PEM:           nodeNetworkStringToTF(cert.PEM),
			PublicKeyType: nodeNetworkStringToTF(cert.PublicKeyType),
			PublicKeyBits: haInt64PtrToTF(cert.PublicKeyBits),
			NotBefore:     haInt64PtrToTF(cert.NotBefore),
			NotAfter:      haInt64PtrToTF(cert.NotAfter),
			SAN:           listStringToTF(cert.SAN),
		})
	}

	defaults, err := d.client.GetVzdumpDefaults(ctx, node, "")
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node", fmt.Sprintf("reading vzdump defaults (GET /nodes/%s/vzdump/defaults) for node %s: %s", node, node, err))
		return
	}
	data.VzdumpDefaults = pveBackupJobsDefaultsFromWire(defaults)

	version, err := d.client.GetNodeVersion(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node", fmt.Sprintf("reading version (GET /nodes/%s/version) for node %s: %s", node, node, err))
		return
	}
	data.PVEVersion = &pveNodeDataSourceVersionModel{
		Release: types.StringValue(version.Release),
		RepoID:  types.StringValue(version.RepoID),
		Version: types.StringValue(version.Version),
	}

	data.ID = types.StringValue(node)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
