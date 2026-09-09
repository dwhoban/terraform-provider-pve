# Copyright (c) HashiCorp, Inc.

# Read the cluster membership record of node pve1.
data "pve_cluster_node" "pve1" {
  node = "pve1"
}

output "pve1_cluster_ip" {
  value = data.pve_cluster_node.pve1.ip
}
