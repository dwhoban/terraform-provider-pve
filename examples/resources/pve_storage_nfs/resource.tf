# Manage an NFS export as a PVE storage definition.
resource "pve_storage_nfs" "media" {
  storage = "media"
  server  = "192.168.1.10"
  export  = "/srv/export/media"

  content = ["images", "iso", "backup"]
  nodes   = "pve1,pve2"
  options = "vers=4.2,soft"
  shared  = true

  prune_backups         = "keep-last=3,keep-monthly=1"
  max_protected_backups = 5
}
