# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Migrates all guests from node pve1 to pve2. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_migrate_all.drain_pve1]
action "pve_node_migrate_all" "drain_pve1" {
  config {
    node   = "pve1"
    target = "pve2"
  }
}
