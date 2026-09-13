resource "pve_vnet_firewall_rules" "vnet0" {
  vnet = "vnet0"
  rules = [
    {
      type    = "in"
      action  = "ACCEPT"
      source  = "+vnet0-allowed"
      comment = "Members of the vnet0 IP set"
    },
    {
      type   = "in"
      action = "DROP"
    },
  ]
}
