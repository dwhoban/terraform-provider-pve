# Copyright (c) HashiCorp, Inc.

resource "pve_ceph_mon" "pve1" {
  node        = "pve1"
  mon_address = "10.0.0.11"
}
