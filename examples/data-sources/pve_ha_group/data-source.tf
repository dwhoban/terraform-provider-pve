# Copyright (c) HashiCorp, Inc.

data "pve_ha_group" "primary" {
  group = "primary"
}
