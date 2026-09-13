data "pve_vm_snapshot" "pre_upgrade" {
  node = "pve1"
  vmid = 100
  name = "snap1"
}
