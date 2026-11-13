data "pve_storage_prune_backups_preview" "example" {
  node    = "pve1"
  storage = "local"
}
