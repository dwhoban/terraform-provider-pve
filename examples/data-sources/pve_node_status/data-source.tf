# Copyright (c) HashiCorp, Inc.

data "pve_node_status" "pve1" {
  node = "pve1"
}

output "pve1_uptime_seconds" {
  value = data.pve_node_status.pve1.uptime
}

output "pve1_kernel" {
  value = data.pve_node_status.pve1.kernel
}
