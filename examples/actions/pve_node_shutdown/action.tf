# Copyright (c) HashiCorp, Inc.

# Destructive: powers off node pve1; it must be powered on manually
# afterwards. Guests running on pve1 are interrupted unless HA relocates
# them. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_shutdown.poweroff_pve1]
action "pve_node_shutdown" "poweroff_pve1" {
  config {
    node = "pve1"
  }
}
