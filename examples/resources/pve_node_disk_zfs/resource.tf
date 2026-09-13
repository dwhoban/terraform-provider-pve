resource "pve_node_disk_zfs" "tank" {
  node        = "pve1"
  name        = "tank"
  raidlevel   = "mirror"
  devices     = ["/dev/sdb", "/dev/sdc"]
  ashift      = 12
  compression = "zstd"
  add_storage = true
}
