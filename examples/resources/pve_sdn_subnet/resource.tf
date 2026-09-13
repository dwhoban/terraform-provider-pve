# A subnet tracks the IP space of a vnet. Run the `pve_sdn_apply` action
# afterwards to activate pending SDN changes.

resource "pve_sdn_subnet" "subnet1" {
  vnet       = "vnet1"
  subnet     = "10.0.0.0-24"
  gateway    = "10.0.0.1"
  snat       = true
  dhcp_range = ["10.0.0.100-10.0.0.200"]
}
