data "pve_ceph_cmd_safety" "example" {
  action  = "stop"
  item_id = "CHANGE_ME"
  node    = "pve1"
  service = "osd"
}
