# Copyright (c) HashiCorp, Inc.

data "pve_file" "debian_iso" {
  node         = "pve1"
  storage      = "local"
  file_name    = "debian-12.iso"
  content_type = "iso"
}

output "debian_iso" {
  value = {
    volid  = data.pve_file.debian_iso.volid
    size   = data.pve_file.debian_iso.size
    format = data.pve_file.debian_iso.format
    ctime  = data.pve_file.debian_iso.ctime
  }
}
