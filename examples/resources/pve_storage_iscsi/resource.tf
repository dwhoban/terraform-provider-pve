# Copyright (c) HashiCorp, Inc.

# Manage an iSCSI target as a PVE storage definition. iSCSI targets are
# used as disks for VMs only.
resource "pve_storage_iscsi" "san" {
  storage = "san"
  portal  = "192.168.1.10:3260"
  target  = "iqn.2000-01.com.example:san.target0"

  content = ["images"]
  nodes   = "pve1,pve2"
}
