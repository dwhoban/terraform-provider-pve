# Copyright (c) HashiCorp, Inc.

# Run a snapshot-mode backup of guests 100 and 101 to the backup storage
# on node pve1 with zstd compression, keeping the ten most recent
# backups, and wait for the vzdump task to finish.
action "pve_backup_run" "nightly_vm_backup" {
  config {
    node     = "pve1"
    mode     = "snapshot"
    compress = "zstd"
    storage  = "backup-nas"
    vmid     = ["100", "101"]
    prune_backups = {
      keep_last = 10
    }
  }
}
