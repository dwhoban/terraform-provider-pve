# Copyright (c) HashiCorp, Inc.

data "pve_acme_plugins" "all" {
  type = "dns"
}
