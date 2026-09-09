# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_network_interfaces" "pve1" {
  node = "pve1"
  type = "bridge"
}

output "pve1_bridges" {
  value = [for i in data.pve_node_network_interfaces.pve1.interfaces : i.iface]
}
