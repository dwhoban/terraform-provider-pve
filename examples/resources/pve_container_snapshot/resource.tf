resource "pve_container_snapshot" "example" {
  node     = "pve1"
  snapname = "CHANGE_ME"
  vmid     = 100
}
