# OSPF SDN fabric with one node member. Changes stay pending until the
# `pve_sdn_apply` action pushes them cluster-wide.
resource "pve_sdn_fabric_ospf" "ospf1" {
  fabric_id = "ospf1"
  ip_prefix = "10.0.0.0/24"
  area      = "0.0.0.0"

  redistribute = [
    {
      source    = "connected"
      route_map = null
    },
  ]

  nodes = [
    {
      node_id = "pve1"
      ip      = "10.0.0.1"
      ip6     = null
      interfaces = [
        {
          name         = "ens19"
          ip           = "10.0.0.1/24"
          ip6          = null
          network_type = "point-to-point"
        },
      ]
    },
  ]
}
