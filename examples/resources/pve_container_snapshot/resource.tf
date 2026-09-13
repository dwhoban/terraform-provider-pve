# Snapshot of container 100 taken before each upgrade. The snapshot name is
# immutable: only the description updates in place.
resource "pve_container_snapshot" "pre_upgrade" {
  node        = "pve1"
  vmid        = 100
  name        = "pre-upgrade"
  description = "Taken before upgrading the container"
}
