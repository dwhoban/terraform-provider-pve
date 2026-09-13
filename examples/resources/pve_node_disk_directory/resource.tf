resource "pve_node_disk_directory" "backup" {
  node           = "pve1"
  name           = "backup"
  device         = "/dev/sdc"
  filesystem     = "ext4"
  add_storage    = true
  cleanup_config = true
  cleanup_disks  = true
}
