# LXC containers on node pve1.
data "pve_containers" "pve1" {
  node = "pve1"
}

output "running_container_ids" {
  value = [
    for ct in data.pve_containers.pve1.containers : ct.vmid if ct.status == "running"
  ]
}
