// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

// Resource and data-source type-name suffixes. Each constant is appended
// to req.ProviderTypeName in the resource's Metadata method, so the full
// Terraform resource type is e.g. `pve_node`
// after the planned provider rename.
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
)
