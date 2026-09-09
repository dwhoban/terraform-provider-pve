# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Reboots container 100 on pve1: shuts it down and starts it again,
# applying pending configuration changes. Waits up to 60 seconds for the
# shutdown phase. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_container_reboot.reboot_ct100]
action "pve_container_reboot" "reboot_ct100" {
  config {
    node    = "pve1"
    vmid    = 100
    timeout = 60
  }
}
