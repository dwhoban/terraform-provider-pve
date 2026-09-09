# Copyright (c) HashiCorp, Inc.

# Request an online (live) migration of the HA-managed VM 100 to node
# pve2. The HA manager performs the migration after the request is
# accepted.
action "pve_ha_resource_migrate" "migrate_vm100" {
  config {
    sid  = "vm:100"
    node = "pve2"
  }
}
