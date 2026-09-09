# Copyright (c) HashiCorp, Inc.

variable "pbs_password" {
  type      = string
  sensitive = true
  default   = "replace-with-pbs-password"
}

# Proxmox Backup Server datastore for VM backups.
resource "pve_storage_pbs" "pbs1" {
  storage       = "pbs1"
  server        = "192.168.1.10"
  datastore     = "store1"
  username      = "backup@pbs"
  password      = var.pbs_password
  fingerprint   = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
  content       = ["backup"]
  prune_backups = "keep-last=3,keep-daily=7"
}
