# Copyright (c) HashiCorp, Inc.

data "pve_firewall_security_group" "web" {
  group = "web"
}
