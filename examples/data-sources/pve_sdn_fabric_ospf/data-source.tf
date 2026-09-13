# Read the OSPF fabric including live per-node state from pve1.
data "pve_sdn_fabric_ospf" "ospf1" {
  fabric_id = "ospf1"
  node      = "pve1"
}
