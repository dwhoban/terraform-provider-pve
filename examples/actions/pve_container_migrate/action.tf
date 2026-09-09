# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Destructive: moves container 100 from pve1 to pve2 using restart
# migration, which stops the container for the move. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_container_migrate.migrate_ct100]
action "pve_container_migrate" "migrate_ct100" {
  config {
    node           = "pve1"
    vmid           = 100
    target         = "pve2"
    target_storage = "1"
    restart        = true
  }
}
