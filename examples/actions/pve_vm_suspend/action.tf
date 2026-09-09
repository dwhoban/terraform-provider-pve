# Copyright (c) HashiCorp, Inc.

# Suspend VM 100 to disk; it resumes on the next start.
action "pve_vm_suspend" "suspend_vm" {
  config {
    node         = "pve1"
    vmid         = 100
    todisk       = true
    statestorage = "local-lvm"
  }
}
