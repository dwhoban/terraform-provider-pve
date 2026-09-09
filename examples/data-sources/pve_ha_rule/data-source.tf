# Copyright (c) HashiCorp, Inc.

data "pve_ha_rule" "keep_db" {
  rule = "keep-db-on-pve1"
}
