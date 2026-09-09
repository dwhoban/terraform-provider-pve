# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node" "pve1" {
  node        = "pve1"
  description = "primary node managed by terraform"

  dns_search = "example.com"
  dns1       = "1.1.1.1"
  dns2       = "1.0.0.1"

  timezone = "Europe/Berlin"

  acme_domain = [
    {
      domain = "pve1.example.com"
    },
  ]
}
