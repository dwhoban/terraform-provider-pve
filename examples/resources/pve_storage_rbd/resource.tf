# Copyright (c) HashiCorp, Inc.

# External Ceph RBD pool for VM images.
resource "pve_storage_rbd" "rbd1" {
  storage       = "rbd1"
  monhost       = "192.168.1.10:6789"
  pool          = "rbd"
  username      = "admin"
  authsupported = "cephx"
  keyring       = var.ceph_keyring
  content       = ["images", "rootdir"]
  krbd          = false
}
