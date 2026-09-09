# Copyright (c) HashiCorp, Inc.

# Proxmox VE API version details of the responding node.

data "pve_version" "current" {}

output "pve_release" {
  value = data.pve_version.current.release
}

output "pve_manager_version" {
  value = data.pve_version.current.version
}
