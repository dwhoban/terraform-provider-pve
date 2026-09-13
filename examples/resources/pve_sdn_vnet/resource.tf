# A vnet is a virtual network bridged into an SDN zone. Run the
# `pve_sdn_apply` action afterwards to activate pending SDN changes.

resource "pve_sdn_vnet" "vnet1" {
  vnet      = "vnet1"
  zone      = "zone1"
  alias     = "guest network"
  tag       = 10
  vlanaware = true
}
