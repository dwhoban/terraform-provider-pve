# Copyright (c) HashiCorp, Inc.
# A controller runs the routing backbone for EVPN zones. Run the
# `pve_sdn_apply` action afterwards to activate pending SDN changes.

resource "pve_sdn_controller" "bgp1" {
  controller = "bgp1"
  type       = "bgp"
  asn        = 65000
  peers      = ["10.0.0.11", "10.0.0.12"]
  loopback   = "lo"
}
