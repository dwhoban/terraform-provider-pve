# Copyright (c) HashiCorp, Inc.

resource "pve_cluster_firewall_rules" "cluster" {
  rules = [
    {
      type    = "in"
      action  = "ACCEPT"
      proto   = "tcp"
      dport   = "8006"
      source  = "10.0.0.0/8"
      iface   = "vmbr0"
      comment = "Web UI from management network"
    },
    {
      type    = "in"
      action  = "ACCEPT"
      proto   = "tcp"
      dport   = "22"
      comment = "SSH"
    },
    {
      type   = "in"
      action = "DROP"
      log    = "warning"
    },
  ]
}
