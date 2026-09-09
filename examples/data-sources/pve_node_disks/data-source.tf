# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_disks" "pve1" {
  node          = "pve1"
  include_smart = true
}

output "pve1_disks" {
  value = data.pve_node_disks.pve1.disks
}
