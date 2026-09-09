# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Destructive: powers off node pve1; it must be powered on manually
# afterwards. Guests running on pve1 are interrupted unless HA relocates
# them. Invoke with:
#   terraform apply -invoke pve_node_shutdown.poweroff_pve1
invoke "pve_node_shutdown" "poweroff_pve1" {
  config {
    node = "pve1"
  }
}
