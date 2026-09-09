# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_hosts" "pve1" {
  node = "pve1"
}

output "pve1_hosts_entries" {
  value = data.pve_node_hosts.pve1.entries
}
