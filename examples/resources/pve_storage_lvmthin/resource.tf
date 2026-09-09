# Copyright (c) HashiCorp, Inc.

resource "pve_storage_lvmthin" "thinstore" {
  id       = "thinstore"
  content  = ["images", "rootdir"]
  vgname   = "pve"
  thinpool = "data"
}
