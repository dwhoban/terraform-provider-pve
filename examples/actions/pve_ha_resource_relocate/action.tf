# Request relocation of the HA-managed container 101 to node pve3. Unlike
# a migrate, this stops the service on the old node and restarts it on the
# target, causing downtime.
action "pve_ha_resource_relocate" "relocate_ct101" {
  config {
    sid  = "ct:101"
    node = "pve3"
  }
}
