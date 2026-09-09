# Copyright (c) HashiCorp, Inc.

# Bulk migrates the given guests to node pve2. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_guest_bulk_migrate.move_vm100]
action "pve_guest_bulk_migrate" "move_vm100" {
  config {
    target = "pve2"
    vms    = [100]
    online = true
  }
}
