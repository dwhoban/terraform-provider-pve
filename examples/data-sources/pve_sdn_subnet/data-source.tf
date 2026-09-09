# Copyright (c) HashiCorp, Inc.

data "pve_sdn_subnet" "subnet1" {
  vnet   = "vnet1"
  subnet = "10.0.0.0-24"
}
