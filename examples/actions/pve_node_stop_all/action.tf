# Copyright (c) HashiCorp, Inc.

# Destructive: stops all guests on node pve1. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_stop_all.stop_pve1]
action "pve_node_stop_all" "stop_pve1" {
  config {
    node       = "pve1"
    timeout    = 180
    force_stop = true
  }
}
