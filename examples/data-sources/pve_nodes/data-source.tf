# Copyright (c) HashiCorp, Inc.

data "pve_nodes" "all" {}

output "cluster_node_names" {
  value = [for n in data.pve_nodes.all.nodes : n.node]
}
