# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Migrates all guests from node pve1 to pve2. Invoke with:
#   terraform apply -invoke pve_node_migrate_all.drain_pve1
invoke "pve_node_migrate_all" "drain_pve1" {
  config {
    node   = "pve1"
    target = "pve2"
  }
}
