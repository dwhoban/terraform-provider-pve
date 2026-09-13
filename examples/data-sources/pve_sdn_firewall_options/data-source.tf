# Firewall options of the SDN vnet1.
data "pve_sdn_firewall_options" "vnet1" {
  vnet = "vnet1"
}
