# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Suspends container 100 on pve1. Upstream marks container suspend as
# experimental; the container state is frozen in memory. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_container_suspend.suspend_ct100]
action "pve_container_suspend" "suspend_ct100" {
  config {
    node = "pve1"
    vmid = 100
  }
}
