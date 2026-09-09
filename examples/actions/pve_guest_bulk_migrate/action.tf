# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Bulk migrates the given guests to node pve2. Invoke with:
#   terraform apply -invoke pve_guest_bulk_migrate.move_vm100
invoke "pve_guest_bulk_migrate" "move_vm100" {
  config {
    target = "pve2"
    vms    = [100]
    online = true
  }
}
