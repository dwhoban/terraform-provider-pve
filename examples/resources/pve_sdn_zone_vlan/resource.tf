# Manage a VLAN SDN zone on top of a local bridge.
resource "pve_sdn_zone_vlan" "zone1" {
  zone   = "zone1"
  bridge = "vmbr0"

  nodes                       = "pve1,pve2"
  bridge_disable_mac_learning = false
}
