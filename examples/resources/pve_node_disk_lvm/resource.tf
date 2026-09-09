# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node_disk_lvm" "data" {
  node           = "pve1"
  name           = "data"
  devices        = ["/dev/sdd"]
  add_storage    = false
  cleanup_config = true
  cleanup_disks  = true
}
