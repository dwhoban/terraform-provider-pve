# Copyright (c) HashiCorp, Inc.

# Requires the QEMU guest agent inside vm 100.
data "pve_vm_agent_info" "vm100" {
  node = "pve1"
  vmid = 100
}

output "guest_hostname" {
  value = data.pve_vm_agent_info.vm100.hostname
}
