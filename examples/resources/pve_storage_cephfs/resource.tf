variable "ceph_keyring" {
  type      = string
  sensitive = true
  default   = "replace-with-ceph-keyring-contents"
}

# External CephFS cluster mounted as VM image/container storage.
resource "pve_storage_cephfs" "cephfs1" {
  storage  = "cephfs1"
  monhost  = "192.168.1.10:6789"
  username = "admin"
  keyring  = var.ceph_keyring
  fs_name  = "cephfs"
  subdir   = "/pve"
  content  = ["images", "rootdir"]
}
