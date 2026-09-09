# Copyright (c) HashiCorp, Inc.

resource "pve_storage_directory" "local" {
  id             = "local"
  path           = "/var/lib/vz"
  content        = ["images", "rootdir", "iso", "vztmpl", "backup", "snippets"]
  create_subdirs = true
}

resource "pve_storage_directory" "nfs_mount" {
  id            = "external"
  path          = "/mnt/external"
  content       = ["backup"]
  is_mountpoint = "yes"
}
