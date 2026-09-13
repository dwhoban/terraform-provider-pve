resource "pve_storage_zfspool" "zfspool" {
  id        = "zfspool"
  content   = ["images", "rootdir"]
  pool      = "rpool/data"
  blocksize = "16k"
  sparse    = false
}
