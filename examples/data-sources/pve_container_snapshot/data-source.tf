# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_container_snapshot" "pre_upgrade" {
  node = "pve1"
  vmid = 100
  name = "pre-upgrade"
}

output "snapshot_taken_at" {
  value = data.pve_container_snapshot.pre_upgrade.snaptime
}
