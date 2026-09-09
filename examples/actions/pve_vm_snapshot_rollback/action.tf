# Copyright (c) HashiCorp, Inc.

# DESTRUCTIVE: revert VM 100 and its disks to snapshot "snap1".
# All data written after the snapshot was taken is lost.
action "pve_vm_snapshot_rollback" "rollback_vm" {
  config {
    node = "pve1"
    vmid = 100
    name = "snap1"
  }
}
