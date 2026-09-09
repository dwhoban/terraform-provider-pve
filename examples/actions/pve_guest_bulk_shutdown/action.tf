# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Destructive: bulk shuts down the given guests cluster-wide. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_guest_bulk_shutdown.stop_vm100]
action "pve_guest_bulk_shutdown" "stop_vm100" {
  config {
    vms     = [100]
    timeout = 180
  }
}
