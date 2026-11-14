resource "pve_ha_rule" "example" {
  resources = ["CHANGE_ME"]
  rule      = "CHANGE_ME"
  type      = "node-affinity"
}
