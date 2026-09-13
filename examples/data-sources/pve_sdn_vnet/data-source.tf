# Read an SDN vnet; set `node` to also fetch the per-node MAC VRF routes.

data "pve_sdn_vnet" "vnet1" {
  vnet = "vnet1"
  node = "pve1"
}
