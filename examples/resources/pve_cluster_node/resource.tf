# Manages cluster membership for node pve1. The provider endpoint must
# target pve1 itself: create calls POST /cluster/config/join on it, which
# joins the node into the existing cluster at 10.0.0.10.
#
# WARNING: create and destroy change cluster membership for every guest,
# container, and storage resource running on the node.
resource "pve_cluster_node" "pve1" {
  node          = "pve1"
  peer_host     = "10.0.0.10"
  peer_password = var.peer_root_password
  fingerprint   = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
  nodeid        = 2
  votes         = 1
}

variable "peer_root_password" {
  type      = string
  sensitive = true
}
