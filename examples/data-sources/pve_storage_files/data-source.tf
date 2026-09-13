data "pve_storage_files" "local_iso" {
  node    = "pve1"
  storage = "local"
  content = "iso"
}

output "local_iso_files" {
  value = data.pve_storage_files.local_iso.files
}
