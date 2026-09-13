# Preview what a prune with this retention would remove from the `local`
# storage on node `pve1`, without deleting anything. The action reports how
# many backups would be removed, kept, protected, or renamed.
action "pve_storage_prune_backups" "preview" {
  config {
    node       = "pve1"
    storage    = "local"
    dry_run    = true
    keep_last  = 3
    keep_daily = 7
  }
}

# Prune for real: keep the 3 most recent and the 7 most recent daily
# backups; everything else following the standard naming scheme is removed.
# The action waits for the prune task to finish.
action "pve_storage_prune_backups" "prune" {
  config {
    node       = "pve1"
    storage    = "local"
    keep_last  = 3
    keep_daily = 7
  }
}

# Only prune backups of one guest:
# action "pve_storage_prune_backups" "single_guest" {
#   config {
#     node    = "pve1"
#     storage = "local"
#     vmid    = 100
#     type    = "qemu"
#     keep_last = 5
#   }
# }
