# Suspends all VMs on node pve1; their memory stays allocated. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_suspend_all.suspend_pve1]
action "pve_node_suspend_all" "suspend_pve1" {
  config {
    node = "pve1"
  }
}
