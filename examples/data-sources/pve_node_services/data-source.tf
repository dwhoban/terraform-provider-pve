# Systemd services managed by PVE on node pve1, with their current states.
data "pve_node_services" "pve1" {
  node = "pve1"
}

output "pve1_service_states" {
  value = { for svc in data.pve_node_services.pve1.services : svc.name => svc.state }
}
