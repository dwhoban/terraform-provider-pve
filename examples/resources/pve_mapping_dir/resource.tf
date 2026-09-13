resource "pve_mapping_dir" "share" {
  id          = "share"
  description = "Media share exposed to guests"

  map = [
    { node = "pve1", path = "/mnt/share" },
    { node = "pve2", path = "/mnt/share" },
  ]
}
