# Copyright (c) HashiCorp, Inc.

# Manage a simple SDN zone with IPAM and DNS backends.
resource "pve_sdn_zone_simple" "zone1" {
  zone  = "zone1"
  nodes = "pve1,pve2"
  mtu   = 1500

  ipam    = "pve"
  dns     = "dns1"
  dnszone = "example.com"
}
