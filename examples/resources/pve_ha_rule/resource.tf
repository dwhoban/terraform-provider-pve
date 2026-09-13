resource "pve_ha_rule" "keep_db" {
  rule      = "keep-db-on-pve1"
  type      = "node-affinity"
  nodes     = ["pve1:2", "pve2:1"]
  resources = ["vm:200"]
  strict    = false
  comment   = "Keep the database on pve1 when available"
}
