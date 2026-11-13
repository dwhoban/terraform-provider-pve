resource "pve_sdn_prefix_list_entry" "example" {
  action  = "permit"
  item_id = "CHANGE_ME"
  prefix  = "CHANGE_ME"
  url_seq = 1
}
