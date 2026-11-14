resource "pve_apt_repository" "ceph" {
  node      = "pve1"
  file_path = "/etc/apt/sources.list.d/ceph.sources"
  index     = 0
  enabled   = false
}
