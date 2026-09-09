# Copyright (c) HashiCorp, Inc.

resource "pve_security_group_firewall_rules" "web" {
  group = "web"
  rules = [
    {
      type   = "in"
      action = "ACCEPT"
      macro  = "HTTP"
    },
    {
      type   = "in"
      action = "ACCEPT"
      macro  = "HTTPS"
    },
  ]
}
