# Manage a VXLAN SDN zone between the nodes' VTEPs.
resource "pve_sdn_zone_vxlan" "zone1" {
  zone  = "zone1"
  peers = "10.0.0.1,10.0.0.2"

  mtu        = 1450
  vxlan_port = 4789
  nodes      = "pve1,pve2"
}
