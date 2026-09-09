# Copyright (c) HashiCorp, Inc.

resource "pve_sdn_route_map" "rm1" {
  route_map_id = "rm1"
  entries = [
    {
      action = "permit"
      match = [
        {
          key   = "ip-address-prefix-list"
          value = "pl1"
        },
      ]
      set = [
        {
          key   = "local-preference"
          value = "200"
        },
      ]
    },
    {
      action = "deny"
    },
  ]
}
