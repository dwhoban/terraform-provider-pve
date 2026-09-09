# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Request an online (live) migration of the HA-managed VM 100 to node
# pve2. The HA manager performs the migration after the request is
# accepted.
action "pve_ha_resource_migrate" "migrate_vm100" {
  config {
    sid  = "vm:100"
    node = "pve2"
  }
}
