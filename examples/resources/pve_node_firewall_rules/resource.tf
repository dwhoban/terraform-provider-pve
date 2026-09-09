# Copyright (c) HashiCorp, Inc.

resource "pve_node_firewall_rules" "pve1" {
  node = "pve1"
  rules = [
    {
      type    = "in"
      action  = "ACCEPT"
      macro   = "SSH"
      comment = "SSH via macro"
    },
    {
      type   = "in"
      action = "DROP"
      log    = "nolog"
    },
  ]
}
