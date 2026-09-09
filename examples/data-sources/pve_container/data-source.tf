# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_container" "ct1" {
  node = "pve1"
  vmid = 100
}

output "ct1_status" {
  value = data.pve_container.ct1.status
}

output "ct1_mount_points" {
  value = data.pve_container.ct1.mount_points
}

# Interface addresses are only discovered while the container is running.
output "ct1_addresses" {
  value = [for iface in data.pve_container.ct1.interfaces : iface.inet]
}
