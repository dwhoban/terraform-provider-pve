# Copyright (c) HashiCorp, Inc.

resource "pve_backup_job" "daily" {
  id       = "daily"
  schedule = "mon..fri 02:00"
  storage  = "local"
  mode     = "snapshot"
  compress = "zstd"
  enabled  = true

  vmid         = ["100", "101"]
  exclude_path = ["/tmp/cache"]

  mailto            = "ops@example.com"
  notification_mode = "auto"

  performance = {
    max_workers = 4
  }

  prune_backups = {
    keep_daily = 7
    keep_last  = 3
  }
}
