data "pve_ha_status" "current" {}

output "ha_quorate" {
  value = data.pve_ha_status.current.quorate
}

output "ha_master_node" {
  value = data.pve_ha_status.current.master_node
}

output "ha_services" {
  value = [for entry in data.pve_ha_status.current.entries : entry.sid if entry.type == "service"]
}
