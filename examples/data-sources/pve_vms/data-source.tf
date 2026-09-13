# List every virtual machine on node pve1.

data "pve_vms" "pve1" {
  node = "pve1"
}

output "running_vmids" {
  value = [for vm in data.pve_vms.pve1.vms : vm.vmid if vm.status == "running"]
}

# Request the full status field set (qmpstatus, running_qemu, pressure
# gauges, and so on) for running VMs.

data "pve_vms" "pve1_full" {
  node = "pve1"
  full = true
}

output "qmp_states" {
  value = { for vm in data.pve_vms.pve1_full.vms : vm.vmid => vm.qmpstatus }
}
