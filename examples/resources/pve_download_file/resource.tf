# Have PVE download an ISO directly from a URL onto the `local` storage.
# The checksum pair is optional but recommended for reproducible runs.
resource "pve_download_file" "alpine_iso" {
  node         = "pve1"
  storage      = "local"
  url          = "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-virt-3.20.3-x86_64.iso"
  file_name    = "alpine-virt-3.20.3-x86_64.iso"
  content_type = "iso"

  checksum           = "b1e0e4214a1b19a07d2a1bcb4e21d0e0c8b9b6a4c5d3e2f1a0b9c8d7e6f5a4b3"
  checksum_algorithm = "sha256"
}

data "pve_download_file" "alpine_iso" {
  node         = "pve1"
  storage      = "local"
  file_name    = "alpine-virt-3.20.3-x86_64.iso"
  content_type = "iso"
}

output "alpine_iso_volid" {
  value = data.pve_download_file.alpine_iso.volid
}
