# Copyright (c) HashiCorp, Inc.

# Cluster membership, quorum, corosync totem settings, and QDevice status.

data "pve_cluster_status" "cluster" {}

output "cluster_name" {
  value = data.pve_cluster_status.cluster.name
}

output "quorate" {
  value = data.pve_cluster_status.cluster.quorate
}

output "online_nodes" {
  value = [for n in data.pve_cluster_status.cluster.nodes : n.name if n.online]
}
