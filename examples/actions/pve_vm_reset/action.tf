# Hard-reset VM 100, like pressing the reset button. The guest OS
# does not shut down cleanly.
action "pve_vm_reset" "reset_vm" {
  config {
    node = "pve1"
    vmid = 100
  }
}
