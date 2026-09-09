# Copyright (c) HashiCorp, Inc.

# Host firewall options of node pve1. One instance per node manages the
# listed options; removing an attribute clears it via the `delete`
# parameter. Destroy only forgets the state.
resource "pve_node_firewall_options" "pve1" {
  node   = "pve1"
  enable = true

  log_level_in     = "info"
  log_level_out    = "nolog"
  nf_conntrack_max = 262144
  nosmurfs         = true
  tcpflags         = true
}
