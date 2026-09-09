# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_storage_files" "local_iso" {
  node    = "pve1"
  storage = "local"
  content = "iso"
}

output "local_iso_files" {
  value = data.pve_storage_files.local_iso.files
}
