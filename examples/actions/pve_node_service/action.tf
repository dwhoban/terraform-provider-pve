# Hard-restart the PVE API proxy on node pve1 and wait for the task to
# finish. Use `reload` instead to reduce interruptions.
action "pve_node_service" "restart_pveproxy" {
  config {
    node      = "pve1"
    service   = "pveproxy"
    operation = "restart"
  }
}
