# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Suspends the given guests cluster-wide to disk. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_guest_bulk_suspend.suspend_vm100]
action "pve_guest_bulk_suspend" "suspend_vm100" {
  config {
    vms     = [100]
    to_disk = true
  }
}
