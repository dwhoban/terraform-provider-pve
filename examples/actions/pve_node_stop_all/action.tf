# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Destructive: stops all guests on node pve1. Invoke with:
#   terraform apply -invoke pve_node_stop_all.stop_pve1
invoke "pve_node_stop_all" "stop_pve1" {
  config {
    node       = "pve1"
    timeout    = 180
    force_stop = true
  }
}
