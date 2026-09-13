# Read the effective configuration and runtime status of one QEMU guest.
data "pve_vm" "web" {
  node = "pve1"
  vmid = 100
}

output "web_status" {
  value = data.pve_vm.web.status
}

output "web_disks" {
  value = data.pve_vm.web.disks
}
