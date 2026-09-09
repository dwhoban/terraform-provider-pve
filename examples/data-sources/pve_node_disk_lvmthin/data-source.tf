# Copyright (c) HashiCorp, Inc.

data "pve_node_disk_lvmthin" "data" {
  node = "pve1"
  name = "data"
}

output "data_thinpool_used_bytes" {
  value = data.pve_node_disk_lvmthin.data.used
}
