resource "pve_ha_resource" "web" {
  sid          = "vm:100"
  state        = "started"
  max_restart  = 2
  max_relocate = 1
  failback     = true
  comment      = "Web frontend kept highly available"
}
