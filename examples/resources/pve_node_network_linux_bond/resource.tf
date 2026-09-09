# Copyright (c) HashiCorp, Inc.

resource "pve_node_network_linux_bond" "bond0" {
  node                  = "pve1"
  iface                 = "bond0"
  slaves                = ["eno1", "eno2"]
  bond_mode             = "802.3ad"
  bond_xmit_hash_policy = "layer3+4"
  autostart             = true
}

resource "pve_node_network_vlan" "vlan100" {
  node            = "pve1"
  iface           = "bond0.100"
  vlan_id         = 100
  vlan_raw_device = "bond0"
  cidr            = "192.168.100.2/24"
  method          = "static"
}
