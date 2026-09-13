# Manage an EVPN SDN zone on a BGP/EVPN controller.
resource "pve_sdn_zone_evpn" "zone1" {
  zone       = "zone1"
  controller = "ctrl1"

  vrf_vxlan                  = 100
  advertise_subnets          = true
  exitnodes                  = "pve1,pve2"
  exitnodes_primary          = "pve1"
  rt_import                  = "65000:100"
  secondary_controllers      = ["ctrl2"]
  disable_arp_nd_suppression = false

  nodes = "pve1,pve2"
}
