// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// This file carries the plumbing shared by the five SDN zone resource/data
// source pairs (pve_sdn_zone_simple, pve_sdn_zone_vlan, pve_sdn_zone_qinq,
// pve_sdn_zone_vxlan, pve_sdn_zone_evpn). Each pair fixes its wire `type`
// and projects its own per-type attributes on top of the shared set the
// pin defines for every zone.

// sdnZoneCommonModel holds the attributes shared by every member of the
// SDN zone family. Family resources and data sources embed it in their
// models.
type sdnZoneCommonModel struct {
	Zone       types.String `tfsdk:"zone"`
	Nodes      types.String `tfsdk:"nodes"`
	MTU        types.Int64  `tfsdk:"mtu"`
	IPAM       types.String `tfsdk:"ipam"`
	DNS        types.String `tfsdk:"dns"`
	DNSZone    types.String `tfsdk:"dnszone"`
	ReverseDNS types.String `tfsdk:"reversedns"`
	DHCP       types.String `tfsdk:"dhcp"`
	Digest     types.String `tfsdk:"digest"`
}

// sdnZoneZoneAttribute renders the family's zone identifier attribute:
// required, and forcing recreation on change because the pin's update verb
// cannot rename a zone.
func sdnZoneZoneAttribute() schema.Attribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The SDN zone object identifier (2 to 8 characters, starting with a letter). Changing this value forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// sdnZoneCommonAttributes returns the schema attributes common to the
// whole family in managed-resource form. Per-type attributes are merged on
// top by each resource.
func sdnZoneCommonAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"zone":  sdnZoneZoneAttribute(),
		"nodes": sdnZoneNodesAttribute(),
		"mtu":   sdnZoneMTUAttribute(),
		"ipam":  sdnZoneIPAMAttribute(),
		"dhcp":  sdnZoneDHCPAttribute(),
		"dns":   sdnZoneDNSAttribute(),
		"reversedns": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "ID of the reverse DNS server (PVE `reversedns`) for this zone.",
		},
		"dnszone": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "DNS domain zone for this zone (PVE `dnszone`), for example `mydomain.com`.",
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The read-only SDN configuration revision (PVE `digest`) of the zone section.",
		},
	}
}

// sdnZoneNodesAttribute renders the shared `nodes` attribute; the pin
// carries it as a comma-separated pve-node-list string.
func sdnZoneNodesAttribute() schema.Attribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Comma-separated list of cluster node names where this zone should be created (PVE `pve-node-list`), for example `pve1,pve2`.",
	}
}

// sdnZoneMTUAttribute renders the shared `mtu` attribute. The pin defines
// no numeric range for it, so it stays free-form.
func sdnZoneMTUAttribute() schema.Attribute {
	return schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "MTU of the zone (PVE `mtu`), used for the created VNet bridges.",
	}
}

// sdnZoneIPAMAttribute renders the shared `ipam` attribute.
func sdnZoneIPAMAttribute() schema.Attribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "ID of the IPAM (PVE `ipam`) managing IP addresses in this zone, for example `pve`.",
	}
}

// sdnZoneDNSAttribute renders the shared `dns` attribute.
func sdnZoneDNSAttribute() schema.Attribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "ID of the DNS API server (PVE `dns`) for this zone.",
	}
}

// sdnZoneDHCPAttribute renders the shared `dhcp` attribute with the pin's
// closed enumeration spelled out.
func sdnZoneDHCPAttribute() schema.Attribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Name of the DHCP server backend for this zone (PVE `dhcp`). Must be one of: `dnsmasq`.",
		Validators: []validator.String{
			stringvalidator.OneOf("dnsmasq"),
		},
	}
}

// sdnZoneCommonComputedAttributes returns the family's shared attributes
// in read form for the data sources: everything except the zone lookup key
// is computed.
func sdnZoneCommonComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"zone": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The SDN zone object identifier to look up (2 to 8 characters, starting with a letter).",
		},
		"nodes": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Comma-separated list of cluster node names where this zone is created (PVE `pve-node-list`).",
		},
		"mtu": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "MTU of the zone (PVE `mtu`), used for the created VNet bridges.",
		},
		"ipam": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "ID of the IPAM (PVE `ipam`) managing IP addresses in this zone.",
		},
		"dhcp": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Name of the DHCP server backend for this zone (PVE `dhcp`). Must be one of: `dnsmasq`.",
		},
		"dns": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "ID of the DNS API server (PVE `dns`) for this zone.",
		},
		"reversedns": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "ID of the reverse DNS server (PVE `reversedns`) for this zone.",
		},
		"dnszone": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "DNS domain zone for this zone (PVE `dnszone`).",
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The read-only SDN configuration revision (PVE `digest`) of the zone section.",
		},
	}
}

// sdnZoneCommonFromModel projects the shared attributes into a wire body.
// Null/unknown fields are omitted so the request JSON stays minimal.
func sdnZoneCommonFromModel(m sdnZoneCommonModel) pveclient.SdnZone {
	body := pveclient.SdnZone{
		Zone: m.Zone.ValueString(),
	}
	if !m.Nodes.IsNull() && !m.Nodes.IsUnknown() {
		body.Nodes = m.Nodes.ValueString()
	}
	if !m.MTU.IsNull() && !m.MTU.IsUnknown() {
		v := m.MTU.ValueInt64()
		body.MTU = &v
	}
	if !m.IPAM.IsNull() && !m.IPAM.IsUnknown() {
		body.IPAM = m.IPAM.ValueString()
	}
	if !m.DNS.IsNull() && !m.DNS.IsUnknown() {
		body.DNS = m.DNS.ValueString()
	}
	if !m.DNSZone.IsNull() && !m.DNSZone.IsUnknown() {
		body.DNSZone = m.DNSZone.ValueString()
	}
	if !m.ReverseDNS.IsNull() && !m.ReverseDNS.IsUnknown() {
		body.ReverseDNS = m.ReverseDNS.ValueString()
	}
	if !m.DHCP.IsNull() && !m.DHCP.IsUnknown() {
		body.DHCP = m.DHCP.ValueString()
	}
	return body
}

// sdnZoneCommonApply writes a fetched configuration into the model; absent
// upstream settings become Terraform nulls so optional attributes do not
// churn between reads and plans.
func sdnZoneCommonApply(s *pveclient.SdnZone, m *sdnZoneCommonModel) {
	m.Zone = types.StringValue(s.Zone)
	m.Nodes = nodeNetworkStringToTF(s.Nodes)
	m.MTU = haInt64PtrToTF(s.MTU)
	m.IPAM = nodeNetworkStringToTF(s.IPAM)
	m.DNS = nodeNetworkStringToTF(s.DNS)
	m.DNSZone = nodeNetworkStringToTF(s.DNSZone)
	m.ReverseDNS = nodeNetworkStringToTF(s.ReverseDNS)
	m.DHCP = nodeNetworkStringToTF(s.DHCP)
	m.Digest = nodeNetworkStringToTF(s.Digest)
}

// sdnZoneCommonDeleteFields returns the PVE wire field names to clear on
// update: optional shared attributes present in state but null in the plan.
func sdnZoneCommonDeleteFields(plan, state sdnZoneCommonModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Nodes, state.Nodes) {
		out = append(out, "nodes")
	}
	if nodeNetworkIntCleared(plan.MTU, state.MTU) {
		out = append(out, "mtu")
	}
	if nodeNetworkStringCleared(plan.IPAM, state.IPAM) {
		out = append(out, "ipam")
	}
	if nodeNetworkStringCleared(plan.DNS, state.DNS) {
		out = append(out, "dns")
	}
	if nodeNetworkStringCleared(plan.DNSZone, state.DNSZone) {
		out = append(out, "dnszone")
	}
	if nodeNetworkStringCleared(plan.ReverseDNS, state.ReverseDNS) {
		out = append(out, "reversedns")
	}
	if nodeNetworkStringCleared(plan.DHCP, state.DHCP) {
		out = append(out, "dhcp")
	}
	return out
}

// sdnZoneGetChecked reads one zone and errors when the upstream type does
// not match the component's fixed type, so a component never silently
// adopts a zone of another kind.
func sdnZoneGetChecked(ctx context.Context, client *pveclient.Client, zone, wantType string) (*pveclient.SdnZone, error) {
	z, err := client.GetSdnZone(ctx, zone)
	if err != nil {
		return nil, err
	}
	if z.Type != wantType {
		return nil, sdnZoneTypeMismatch(zone, wantType, z.Type)
	}
	return z, nil
}

// sdnZoneTypeMismatch reports an error for a zone whose upstream type does
// not match the fixed type of the component reading it.
func sdnZoneTypeMismatch(zone, want, got string) error {
	return fmt.Errorf("sdn zone %s has upstream type %q, want %q; this component only manages %s zones", zone, got, want, want)
}

// sdnZoneBoolCleared reports a bool attr null in plan but set in state.
func sdnZoneBoolCleared(plan, state types.Bool) bool {
	return plan.IsNull() && !state.IsNull()
}

// sdnZoneListCleared reports a list attr null in plan but set in state.
func sdnZoneListCleared(plan, state types.List) bool {
	return plan.IsNull() && !state.IsNull()
}

// sdnZoneDataSourceAttributes converts a resource-side zone attribute set
// (leaf String/Bool/Int64/List attributes only) into its data-source
// equivalent. RequiresReplace plan modifiers are intentionally dropped:
// data sources have no plan. Any unsupported attribute type is a schema
// bug and panics so the schema tests fail loudly.
func sdnZoneDataSourceAttributes(in map[string]schema.Attribute) map[string]datasourceschema.Attribute {
	out := make(map[string]datasourceschema.Attribute, len(in))
	for name, attr := range in {
		switch a := attr.(type) {
		case schema.StringAttribute:
			out[name] = datasourceschema.StringAttribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		case schema.BoolAttribute:
			out[name] = datasourceschema.BoolAttribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		case schema.Int64Attribute:
			out[name] = datasourceschema.Int64Attribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		case schema.ListAttribute:
			out[name] = datasourceschema.ListAttribute{
				ElementType:         a.ElementType,
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		default:
			panic(fmt.Sprintf("sdnZoneDataSourceAttributes: attribute %q has unsupported type %T", name, attr))
		}
	}
	return out
}
