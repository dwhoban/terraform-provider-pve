# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_disk_directory" "backup" {
  node = "pve1"
  name = "backup"
}

output "backup_mount" {
  value = {
    path     = data.pve_node_disk_directory.backup.path
    device   = data.pve_node_disk_directory.backup.device
    type     = data.pve_node_disk_directory.backup.type
    unitfile = data.pve_node_disk_directory.backup.unitfile
  }
}
