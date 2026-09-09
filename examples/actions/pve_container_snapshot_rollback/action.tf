# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Destructive: rolls container 100 on pve1 back to the pre-upgrade
# snapshot, discarding every change made after it was taken, and starts the
# container afterwards. Invoke with:
#   terraform apply -invoke pve_container_snapshot_rollback.rollback_ct100
invoke "pve_container_snapshot_rollback" "rollback_ct100" {
  config {
    node  = "pve1"
    vmid  = 100
    name  = "pre-upgrade"
    start = true
  }
}
