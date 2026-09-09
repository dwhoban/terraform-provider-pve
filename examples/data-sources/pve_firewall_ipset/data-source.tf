# Copyright (c) HashiCorp, Inc.

data "pve_firewall_ipset" "management" {
  name = "management"
}
