# Copyright (c) HashiCorp, Inc.

# Reboot VM 100 on node pve1, waiting up to 60 seconds for the
# shutdown half of the reboot.
action "pve_vm_reboot" "reboot_web" {
  config {
    node    = "pve1"
    vmid    = 100
    timeout = 60
  }
}
