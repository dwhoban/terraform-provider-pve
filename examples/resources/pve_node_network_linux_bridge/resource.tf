# Copyright (c) HashiCorp, Inc.

resource "pve_node_network_linux_bridge" "vmbr0" {
  node              = "pve1"
  iface             = "vmbr0"
  bridge_ports      = "eno1"
  bridge_vlan_aware = true
  autostart         = true
  cidr              = "10.0.0.1/24"
  gateway           = "10.0.0.254"
  method            = "static"
}
