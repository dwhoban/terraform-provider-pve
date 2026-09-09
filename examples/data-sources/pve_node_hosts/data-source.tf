# Copyright (c) HashiCorp, Inc.

data "pve_node_hosts" "pve1" {
  node = "pve1"
}

output "pve1_hosts_entries" {
  value = data.pve_node_hosts.pve1.entries
}
