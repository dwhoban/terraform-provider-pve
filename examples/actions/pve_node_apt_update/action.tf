# Copyright (c) HashiCorp, Inc.

# Refresh the package index of node pve1 (apt-get update) and wait for the
# task to finish.
action "pve_node_apt_update" "refresh_pve1" {
  config {
    node  = "pve1"
    quiet = true
  }
}
