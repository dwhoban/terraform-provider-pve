# OpenFabric SDN fabric with one node member. Changes stay pending until
# the `pve_sdn_apply` action pushes them cluster-wide.
resource "pve_sdn_fabric_openfabric" "openfabric1" {
  fabric_id      = "ofab1"
  ip_prefix      = "10.0.0.0/24"
  hello_interval = 10

  nodes = [
    {
      node_id = "pve1"
      ip      = "10.0.0.1"
      ip6     = null
      interfaces = [
        {
          name             = "ens19"
          ip               = "10.0.0.1/24"
          ip6              = null
          hello_multiplier = 3
        },
      ]
    },
  ]
}
