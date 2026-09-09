# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_nodes" "all" {}

output "cluster_node_names" {
  value = [for n in data.pve_nodes.all.nodes : n.node]
}
