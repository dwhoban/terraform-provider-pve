# Copyright (c) HashiCorp, Inc.

resource "pve_ceph_pool" "rbd" {
  node              = "pve1"
  name              = "rbd"
  size              = 3
  min_size          = 2
  pg_num            = 128
  pg_autoscale_mode = "on"
  application       = "rbd"
  add_storages      = true
}
