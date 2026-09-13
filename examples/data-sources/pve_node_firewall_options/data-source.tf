# Host firewall options of node pve1.
data "pve_node_firewall_options" "pve1" {
  node = "pve1"
}
