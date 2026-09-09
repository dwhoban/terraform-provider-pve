# Copyright (c) HashiCorp, Inc.

# Destructive: bulk shuts down the given guests cluster-wide. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_guest_bulk_shutdown.stop_vm100]
action "pve_guest_bulk_shutdown" "stop_vm100" {
  config {
    vms     = [100]
    timeout = 180
  }
}
