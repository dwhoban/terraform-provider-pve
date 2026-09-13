resource "pve_replication" "vm100" {
  id       = "100-0"
  target   = "pve2"
  schedule = "*/15"
  rate     = 50.5
  comment  = "Replicate vm100 to pve2"
}
