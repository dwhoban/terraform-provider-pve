# Copyright (c) HashiCorp, Inc.

# Firewall options of the qemu VM 100 on node pve1. One instance per guest
# manages the listed options; removing an attribute clears it via the
# `delete` parameter. Destroy only forgets the state.
resource "pve_guest_firewall_options" "vm100" {
  node       = "pve1"
  guest_type = "qemu"
  vmid       = 100

  enable       = true
  macfilter    = true
  ipfilter     = true
  ndp          = true
  policy_in    = "DROP"
  policy_out   = "ACCEPT"
  log_level_in = "emerg"
}
