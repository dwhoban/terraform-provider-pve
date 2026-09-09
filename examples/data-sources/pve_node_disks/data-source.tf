# Copyright (c) HashiCorp, Inc.

data "pve_node_disks" "pve1" {
  node          = "pve1"
  include_smart = true
}

output "pve1_disks" {
  value = data.pve_node_disks.pve1.disks
}
