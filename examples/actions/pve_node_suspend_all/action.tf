# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Suspends all VMs on node pve1; their memory stays allocated. Invoke with:
#   terraform apply -invoke pve_node_suspend_all.suspend_pve1
invoke "pve_node_suspend_all" "suspend_pve1" {
  config {
    node = "pve1"
  }
}
