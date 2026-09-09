# Copyright (c) HashiCorp, Inc.

# Manage a QinQ SDN zone with a service-VLAN tag.
resource "pve_sdn_zone_qinq" "zone1" {
  zone          = "zone1"
  bridge        = "vmbr0"
  tag           = 100
  vlan_protocol = "802.1ad"

  nodes = "pve1,pve2"
}
