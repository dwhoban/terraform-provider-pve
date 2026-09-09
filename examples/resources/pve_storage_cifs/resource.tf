# Copyright (c) HashiCorp, Inc.

# Manage a CIFS/SMB share as a PVE storage definition.
resource "pve_storage_cifs" "archive" {
  storage    = "archive"
  server     = "192.168.1.10"
  share      = "archive"
  username   = "backup"
  domain     = "CORP"
  smbversion = "3.11"

  content = ["backup"]

  # PVE never returns the password on reads; it is sent on every apply
  # that carries it.
  password = var.archive_password
}

variable "archive_password" {
  type      = string
  sensitive = true
}
