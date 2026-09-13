resource "pve_firewall_alias" "office" {
  name    = "office"
  cidr    = "203.0.113.0/24"
  comment = "Office network"
}
