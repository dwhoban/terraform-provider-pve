# List every resource in the Proxmox VE cluster.

data "pve_cluster_resources" "all" {}

output "running_guests" {
  value = [for r in data.pve_cluster_resources.all.resources : r.id if r.status == "running"]
}

# Only guest resources, filtered server-side.

data "pve_cluster_resources" "vms" {
  type = "vm"
}

output "vmids" {
  value = [for r in data.pve_cluster_resources.vms.resources : r.vmid]
}
