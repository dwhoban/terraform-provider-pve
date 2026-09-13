resource "pve_node_disk_lvm" "data" {
  node           = "pve1"
  name           = "data"
  devices        = ["/dev/sdd"]
  add_storage    = false
  cleanup_config = true
  cleanup_disks  = true
}
