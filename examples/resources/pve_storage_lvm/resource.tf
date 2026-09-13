resource "pve_storage_lvm" "vmstore" {
  id      = "vmstore"
  content = ["images", "rootdir"]
  vgname  = "pve"

  nodes = ["pve1"]

  saferemove = false
}
