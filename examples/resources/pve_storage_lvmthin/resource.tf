resource "pve_storage_lvmthin" "thinstore" {
  id       = "thinstore"
  content  = ["images", "rootdir"]
  vgname   = "pve"
  thinpool = "data"
}
