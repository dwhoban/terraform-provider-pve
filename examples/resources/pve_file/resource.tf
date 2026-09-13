# Upload a local ISO image to the `local` storage on node pve1.
resource "pve_file" "debian_iso" {
  node         = "pve1"
  storage      = "local"
  file_name    = "debian-12.iso"
  content_type = "iso"
  source       = "${path.module}/files/debian-12.iso"
}

# Read back the stored volume, e.g. to feed its volume ID into a guest.
data "pve_file" "debian_iso" {
  node         = "pve1"
  storage      = "local"
  file_name    = "debian-12.iso"
  content_type = "iso"
}

output "debian_iso_volid" {
  value = data.pve_file.debian_iso.volid
}
