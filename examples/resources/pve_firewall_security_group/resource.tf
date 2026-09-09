# Copyright (c) HashiCorp, Inc.

resource "pve_firewall_security_group" "web" {
  group   = "web"
  comment = "Web server rules"
}
