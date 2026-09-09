# Copyright (c) HashiCorp, Inc.

# Datacenter-wide cluster options. This is a cluster singleton: one
# instance manages every listed option, and removing an attribute clears it
# on the cluster. Destroy only forgets the state.
resource "pve_cluster_options" "options" {
  email_from = "pve@example.com"
  console    = "xtermjs"
  language   = "en"
  mac_prefix = "BC:24:11"

  migration = {
    type    = "secure"
    network = "10.0.0.0/24"
  }

  ha = {
    shutdown_policy = "conditional"
  }

  bwlimit = {
    move    = 100
    restore = 200
  }
}
