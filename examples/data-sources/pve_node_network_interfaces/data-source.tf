data "pve_node_network_interfaces" "pve1" {
  node = "pve1"
  type = "bridge"
}

output "pve1_bridges" {
  value = [for i in data.pve_node_network_interfaces.pve1.interfaces : i.iface]
}
