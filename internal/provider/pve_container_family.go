// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// containerMountPointModel is the shared nested mount point model of
// pve_container resources and data sources.
type containerMountPointModel struct {
	ID         types.String `tfsdk:"id"`
	Mountpoint types.String `tfsdk:"mountpoint"`
	Storage    types.String `tfsdk:"storage"`
	Volume     types.String `tfsdk:"volume"`
	Size       types.String `tfsdk:"size"`
	ACL        types.Bool   `tfsdk:"acl"`
	Backup     types.Bool   `tfsdk:"backup"`
	ReadOnly   types.Bool   `tfsdk:"ro"`
}

// containerInterfaceModel is the shared nested interface model backed by
// GET .../interfaces (guest IP discovery).
type containerInterfaceModel struct {
	Name   types.String `tfsdk:"name"`
	HWAddr types.String `tfsdk:"hwaddr"`
	Inet   types.String `tfsdk:"inet"`
	Inet6  types.String `tfsdk:"inet6"`
}

// containerMountPointFieldSpec describes one mount point nested attribute;
// the same descriptions feed the resource and data source schemas.
type containerMountPointFieldSpec struct {
	name        string
	description string
	isBool      bool
	required    bool
}

var containerMountPointFieldSpecs = []containerMountPointFieldSpec{
	{name: "id", description: "Mount point config key: `rootfs` or `mp0`..`mp255`.", required: true},
	{name: "mountpoint", description: "Path inside the container where the volume is mounted. Required for `mpN` entries; must be empty for `rootfs` (the pin defines no `mp=` option there)."},
	{name: "storage", description: "Storage holding the volume. Set together with `size` to allocate a new volume, or with `volume` to reference an existing one. Leave unset for bind mounts given via `volume`."},
	{name: "volume", description: "Existing volume id or host path (bind mount). Takes precedence over storage allocation when `storage` is unset."},
	{name: "size", description: "Volume size in PVE disk-size notation (for example `8G`). Growth-only: shrinking is rejected and the growth is applied via the resize verb. Required when allocating a new volume from `storage`."},
	{name: "acl", description: "Explicitly enable or disable ACL support.", isBool: true},
	{name: "backup", description: "Whether to include the mount point in backups (only used for volume mount points).", isBool: true},
	{name: "ro", description: "Read-only mount point.", isBool: true},
}

// containerMountPointNestedAttributes returns the attribute set of the
// mount point nested object for the managed resource (Optional) or, with
// computed set, for data sources (Computed).
func containerMountPointNestedAttributes(computed bool) map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(containerMountPointFieldSpecs))
	for _, spec := range containerMountPointFieldSpecs {
		if spec.isBool {
			out[spec.name] = schema.BoolAttribute{
				Optional:            !computed,
				Computed:            computed,
				MarkdownDescription: spec.description,
			}
			continue
		}
		out[spec.name] = schema.StringAttribute{
			Required:            spec.required && !computed,
			Optional:            !spec.required && !computed,
			Computed:            computed,
			MarkdownDescription: spec.description,
		}
	}
	return out
}

// containerMountPointDSNestedAttributes returns the mount point nested
// attribute set for data-source schemas (all attributes Computed).
func containerMountPointDSNestedAttributes() map[string]dsschema.Attribute {
	out := make(map[string]dsschema.Attribute, len(containerMountPointFieldSpecs))
	for _, spec := range containerMountPointFieldSpecs {
		if spec.isBool {
			out[spec.name] = dsschema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: spec.description,
			}
			continue
		}
		out[spec.name] = dsschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: spec.description,
		}
	}
	return out
}

// containerInterfaceFieldSpecs lists the interface nested attributes.
var containerInterfaceFieldSpecs = []containerMountPointFieldSpec{
	{name: "name", description: "Interface name as seen inside the container."},
	{name: "hwaddr", description: "Interface MAC address."},
	{name: "inet", description: "IPv4 address with prefix length, when assigned."},
	{name: "inet6", description: "IPv6 address with prefix length, when assigned."},
}

// containerInterfaceNestedAttributes returns the attribute set of the
// interface nested object for managed resources (all Computed).
func containerInterfaceNestedAttributes() map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(containerInterfaceFieldSpecs))
	for _, spec := range containerInterfaceFieldSpecs {
		out[spec.name] = schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: spec.description,
		}
	}
	return out
}

// containerInterfaceDSNestedAttributes returns the interface nested
// attribute set for data-source schemas (all Computed).
func containerInterfaceDSNestedAttributes() map[string]dsschema.Attribute {
	out := make(map[string]dsschema.Attribute, len(containerInterfaceFieldSpecs))
	for _, spec := range containerInterfaceFieldSpecs {
		out[spec.name] = dsschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: spec.description,
		}
	}
	return out
}

// containerMountPointFromWire maps a parsed config entry into the nested
// model, splitting `storage:volid` into its parts.
func containerMountPointFromWire(id string, mp pveclient.LxcMountPoint) containerMountPointModel {
	return containerMountPointModel{
		ID:         types.StringValue(id),
		Mountpoint: nodeNetworkStringToTF(mp.Mountpoint),
		Storage:    nodeNetworkStringToTF(mp.Storage()),
		Volume:     nodeNetworkStringToTF(mp.VolumeID()),
		Size:       nodeNetworkStringToTF(mp.Size),
		ACL:        nodeNetworkBoolPtrToTF(mp.ACL),
		Backup:     nodeNetworkBoolPtrToTF(mp.Backup),
		ReadOnly:   nodeNetworkBoolPtrToTF(mp.ReadOnly),
	}
}

// containerWireVolume renders the model's volume reference: `storage:volid`
// when both are set, the raw `volume` (bind mount or path) when storage is
// unset, and the allocation form `storage:size` when only storage and size
// are set.
func containerWireVolume(m containerMountPointModel) string {
	storage := m.Storage.ValueString()
	volume := m.Volume.ValueString()
	switch {
	case storage != "" && volume != "":
		return storage + ":" + volume
	case storage != "":
		return storage + ":" + m.Size.ValueString()
	default:
		return volume
	}
}

// containerMountPointToWire renders the model into a rootfs/mpN property
// string for create and update PUTs. Size is deliberately not rendered:
// allocation rides the `storage:size` volume form and growth rides the
// resize verb, matching the pin's read-only size option.
func containerMountPointToWire(m containerMountPointModel) pveclient.LxcMountPoint {
	return pveclient.LxcMountPoint{
		Volume:     containerWireVolume(m),
		Mountpoint: m.Mountpoint.ValueString(),
		ACL:        containerBoolPtr(m.ACL),
		Backup:     containerBoolPtr(m.Backup),
		ReadOnly:   containerBoolPtr(m.ReadOnly),
		Extra:      map[string]string{},
	}
}

// containerBoolPtr converts a types.Bool into a *bool, nil for null or
// unknown.
func containerBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// containerMountPointSortKey orders ids rootfs first, then mpN by number.
func containerMountPointSortKey(id string) (int, int) {
	if id == "rootfs" {
		return 0, 0
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(id, "mp")); err == nil {
		return 1, n
	}
	return 2, 0
}

// containerMountPointsFromConfig collects rootfs plus the mpN entries of a
// config read into the ordered nested list.
func containerMountPointsFromConfig(cfg *pveclient.LxcConfig) []containerMountPointModel {
	ids := make([]string, 0, len(cfg.MountPoints)+1)
	for id := range cfg.MountPoints {
		ids = append(ids, id)
	}
	if cfg.Rootfs != nil {
		ids = append(ids, "rootfs")
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, ni := containerMountPointSortKey(ids[i])
		pj, nj := containerMountPointSortKey(ids[j])
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})
	out := make([]containerMountPointModel, 0, len(ids))
	for _, id := range ids {
		if id == "rootfs" {
			out = append(out, containerMountPointFromWire(id, *cfg.Rootfs))
			continue
		}
		out = append(out, containerMountPointFromWire(id, cfg.MountPoints[id]))
	}
	return out
}

// containerMountPointModelsByIndex maps the nested list by config key.
func containerMountPointModelsByIndex(models []containerMountPointModel) map[string]containerMountPointModel {
	out := make(map[string]containerMountPointModel, len(models))
	for _, m := range models {
		out[m.ID.ValueString()] = m
	}
	return out
}

// containerDiskSizeBytes parses a PVE disk-size string ("8G", "+4G",
// "1073741824") into bytes. The second result is false for unparsable
// values.
func containerDiskSizeBytes(s string) (float64, bool) {
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0, false
	}
	multiplier := 1.0
	switch upper := strings.ToUpper(s[len(s)-1:]); upper {
	case "K":
		multiplier, s = 1024, s[:len(s)-1]
	case "M":
		multiplier, s = 1024*1024, s[:len(s)-1]
	case "G":
		multiplier, s = 1024*1024*1024, s[:len(s)-1]
	case "T":
		multiplier, s = 1024*1024*1024*1024, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v * multiplier, true
}

// containerPendingKeys returns the config keys with a pending value or a
// pending delete.
func containerPendingKeys(pending []pveclient.LxcPendingChange) []string {
	keys := make([]string, 0, len(pending))
	for _, change := range pending {
		if change.Pending != nil || (change.Delete != nil && *change.Delete != 0) {
			keys = append(keys, change.Key)
		}
	}
	return keys
}

// containerInterfacesFromWire maps the IP discovery rows into the nested
// model.
func containerInterfacesFromWire(ifaces []pveclient.LxcInterface) []containerInterfaceModel {
	out := make([]containerInterfaceModel, 0, len(ifaces))
	for _, iface := range ifaces {
		out = append(out, containerInterfaceModel{
			Name:   types.StringValue(iface.Name),
			HWAddr: nodeNetworkStringToTF(iface.HWAddr),
			Inet:   nodeNetworkStringToTF(iface.Inet),
			Inet6:  nodeNetworkStringToTF(iface.Inet6),
		})
	}
	return out
}

// containerConfigureResource extracts the configured *pveclient.Client from
// a resource ConfigureRequest. Nil provider data leaves the resource
// unconfigured (unit tests).
func containerConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// containerConfigureDataSource extracts the configured *pveclient.Client
// from a data-source ConfigureRequest. Nil provider data leaves the data
// source unconfigured (unit tests).
func containerConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}
