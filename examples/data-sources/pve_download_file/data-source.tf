data "pve_download_file" "alpine_iso" {
  node         = "pve1"
  storage      = "local"
  file_name    = "alpine-virt-3.20.3-x86_64.iso"
  content_type = "iso"
}

output "alpine_iso" {
  value = {
    volid  = data.pve_download_file.alpine_iso.volid
    size   = data.pve_download_file.alpine_iso.size
    format = data.pve_download_file.alpine_iso.format
    ctime  = data.pve_download_file.alpine_iso.ctime
  }
}
