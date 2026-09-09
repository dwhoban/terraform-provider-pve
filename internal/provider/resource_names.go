// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

// Resource and data-source type-name suffixes. Each constant is appended
// to req.ProviderTypeName in the resource's Metadata method, so the full
// Terraform resource type is e.g. `pve_node`.
//
// Centralizing these strings prevents drift between the implementation
// files (where they are set on resp.TypeName) and the test files (where
// they are asserted against). Tests should reference these constants
// rather than duplicating the literal strings.
const (
	TypeNamePveNode                   = "node"
	TypeNamePveNodeNetworkLinuxBridge = "node_network_linux_bridge"
	TypeNamePveNodeNetworkLinuxBond   = "node_network_linux_bond"
	TypeNamePveNodeNetworkVlan        = "node_network_vlan"
	TypeNamePveNodeDiskZFS            = "node_disk_zfs"
	TypeNamePveNodeDiskLVM            = "node_disk_lvm"
	TypeNamePveNodes                  = "nodes"
	TypeNamePveNodeStatus             = "node_status"
	TypeNamePveNodeDisks              = "node_disks"
	TypeNamePveNodeNetworkInterfaces  = "node_network_interfaces"
	// Phase 1 — access control, cluster core, HA.
	TypeNamePveUser             = "user"
	TypeNamePveUserToken        = "user_token"
	TypeNamePveGroup            = "group"
	TypeNamePveRole             = "role"
	TypeNamePveAcl              = "acl"
	TypeNamePvePermissions      = "permissions"
	TypeNamePveRealmLdap        = "realm_ldap"
	TypeNamePveRealmAd          = "realm_ad"
	TypeNamePveRealmOpenid      = "realm_openid"
	TypeNamePveRealms           = "realms"
	TypeNamePveRealmSync        = "realm_sync"
	TypeNamePveRealmSyncJob     = "realm_sync_job"
	TypeNamePveClusterResources = "cluster_resources"
	TypeNamePveClusterStatus    = "cluster_status"
	TypeNamePveTasks            = "tasks"
	TypeNamePveNextId           = "next_id"
	TypeNamePveClusterNode      = "cluster_node"
	TypeNamePveClusterOptions   = "cluster_options"
	TypeNamePveHaStatus         = "ha_status"
	TypeNamePveHaArm            = "ha_arm"
	TypeNamePveHaGroup          = "ha_group"
	TypeNamePveHaResource       = "ha_resource"
	TypeNamePveHaRule           = "ha_rule"
	TypeNamePveVersion          = "version"
	// Phase 2 — firewall.
	TypeNamePveClusterFirewallOptions     = "cluster_firewall_options"
	TypeNamePveFirewallAlias              = "firewall_alias"
	TypeNamePveFirewallIpset              = "firewall_ipset"
	TypeNamePveFirewallSecurityGroup      = "firewall_security_group"
	TypeNamePveClusterFirewallRules       = "cluster_firewall_rules"
	TypeNamePveNodeFirewallRules          = "node_firewall_rules"
	TypeNamePveGuestFirewallRules         = "guest_firewall_rules"
	TypeNamePveSecurityGroupFirewallRules = "security_group_firewall_rules"
	TypeNamePveVnetFirewallRules          = "vnet_firewall_rules"
	TypeNamePveNodeFirewallOptions        = "node_firewall_options"
	TypeNamePveGuestFirewallOptions       = "guest_firewall_options"
	TypeNamePveSdnFirewallOptions         = "sdn_firewall_options"
)
