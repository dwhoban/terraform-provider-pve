# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_node_subscription" "pve1" {
  node = "pve1"
}

output "subscription_status" {
  value = data.pve_node_subscription.pve1.status
}
