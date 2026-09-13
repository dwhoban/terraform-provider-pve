resource "pve_firewall_ipset" "management" {
  name    = "management"
  comment = "Admin hosts"
  cidrs = [
    "10.0.0.1",
    "192.168.1.0/24",
    "+office",
  ]
}
