data "pve_backup_jobs" "all" {
  # Optionally pin the vzdump defaults lookup to one node:
  # node = "pve1"
}
