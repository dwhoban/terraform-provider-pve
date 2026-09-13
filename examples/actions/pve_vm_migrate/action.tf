# Live-migrate VM 100 from pve1 to pve2.
action "pve_vm_migrate" "migrate_vm" {
  config {
    node   = "pve1"
    vmid   = 100
    target = "pve2"
    online = true
  }
}
