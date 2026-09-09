# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node_disk_lvmthin" "data" {
  node           = "pve1"
  name           = "data"
  device         = "/dev/sdb"
  add_storage    = true
  cleanup_config = true
  cleanup_disks  = true
}
