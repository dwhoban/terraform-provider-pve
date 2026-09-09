# Copyright (c) HashiCorp, Inc.

resource "pve_ceph_osd" "sdb" {
  node               = "pve1"
  device             = "/dev/sdb"
  crush_device_class = "hdd"
  encrypted          = false
}
