data "pve_node_storages" "pve1" {
  node = "pve1"
}

output "pve1_storages" {
  value = data.pve_node_storages.pve1.storages
}
