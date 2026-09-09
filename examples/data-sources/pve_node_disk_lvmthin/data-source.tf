# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_disk_lvmthin" "data" {
  node = "pve1"
  name = "data"
}

output "data_thinpool_used_bytes" {
  value = data.pve_node_disk_lvmthin.data.used
}
