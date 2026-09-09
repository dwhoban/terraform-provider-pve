# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Starts every onboot guest on node pve1. Invoke with:
#   terraform apply -invoke pve_node_start_all.start_pve1
invoke "pve_node_start_all" "start_pve1" {
  config {
    node = "pve1"
  }
}
