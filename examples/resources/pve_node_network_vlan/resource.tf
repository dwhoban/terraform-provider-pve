resource "pve_node_network_vlan" "vlan100" {
  node            = "pve1"
  iface           = "vmbr0.100"
  vlan_id         = 100
  vlan_raw_device = "vmbr0"
  cidr            = "192.168.100.2/24"
  method          = "static"
}
