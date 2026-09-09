# Copyright (c) HashiCorp, Inc.

resource "pve_sdn_prefix_list" "pl1" {
  id = "pl1"
  entries = [
    {
      action = "permit"
      prefix = "10.0.0.0/8"
    },
    {
      action = "deny"
      prefix = "10.0.0.0/8"
      ge     = 16
      le     = 32
    },
  ]
}
