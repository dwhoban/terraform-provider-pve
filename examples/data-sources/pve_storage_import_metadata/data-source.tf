data "pve_storage_import_metadata" "example" {
  node    = "pve1"
  storage = "local"
  volume  = "CHANGE_ME"
}
