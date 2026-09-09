# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Refresh the subscription info on node pve1, contacting the Proxmox
# subscription server even if the local cache is still valid.
action "pve_subscription_refresh" "refresh_pve1" {
  config {
    node  = "pve1"
    force = true
  }
}
