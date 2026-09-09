# Copyright (c) HashiCorp, Inc.

# Cluster-wide firewall options. This is a cluster singleton: one instance
# manages every listed option, and removing an attribute clears it on the
# cluster via the `delete` parameter. Destroy only forgets the state.
resource "pve_cluster_firewall_options" "cluster" {
  enable         = 1
  ebtables       = true
  policy_in      = "ACCEPT"
  policy_out     = "ACCEPT"
  policy_forward = "DROP"

  log_ratelimit = "enable=1,burst=5,rate=1/second"
}
