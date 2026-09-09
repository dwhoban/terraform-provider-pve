# Copyright (c) HashiCorp, Inc.

resource "pve_ha_group" "primary" {
  group      = "primary"
  nodes      = ["pve1:2", "pve2:1"]
  restricted = true
  nofailback = false
  comment    = "Prefer pve1, fail over to pve2"
}
