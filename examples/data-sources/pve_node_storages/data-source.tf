# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_storages" "pve1" {
  node = "pve1"
}

output "pve1_storages" {
  value = data.pve_node_storages.pve1.storages
}
