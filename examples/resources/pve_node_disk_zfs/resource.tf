# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node_disk_zfs" "tank" {
  node        = "pve1"
  name        = "tank"
  raidlevel   = "mirror"
  devices     = ["/dev/sdb", "/dev/sdc"]
  ashift      = 12
  compression = "zstd"
  add_storage = true
}
