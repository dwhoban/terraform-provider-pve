# Copyright (c) HashiCorp, Inc.

# Starts every onboot guest on node pve1. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_start_all.start_pve1]
action "pve_node_start_all" "start_pve1" {
  config {
    node = "pve1"
  }
}
