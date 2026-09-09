# Copyright (c) HashiCorp, Inc.
# An IPAM plugin tracks IP address allocation for SDN zones and vnets.
# Run the `pve_sdn_apply` action afterwards to activate pending changes.

resource "pve_sdn_ipam" "netbox1" {
  ipam  = "netbox1"
  type  = "netbox"
  url   = "https://netbox.example/api"
  token = var.netbox_token
}

variable "netbox_token" {
  type      = string
  sensitive = true
}
