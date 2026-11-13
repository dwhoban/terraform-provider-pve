# On the seed node (requires root@pam credentials).
resource "pve_cluster" "prod" {
  clustername = "pve-prod"

  # link[0] is link0, link[1] is link1, ... (up to link7). The address is
  # the corosync link address; PVE writes it without a key.
  link {
    address = "10.0.0.2"
  }
}

# The join information a peer node needs (fingerprint, allowed IPs, totem
# configuration), read from the cluster.
data "pve_cluster_join_info" "seed" {}

# On each peer node's separate root module, using the fingerprint and peer
# address from pve_cluster_join_info:
action "pve_cluster_join" "peer" {
  config {
    hostname    = "10.0.0.2"
    fingerprint = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD"
    password    = "CHANGE_ME-root-password"
  }
}

resource "terraform_data" "peer_join" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_cluster_join.peer]
    }
  }
}

# Removing a node from the cluster (run on any remaining member).
action "pve_cluster_node_remove" "old" {
  config {
    node = "pve3"
  }
}
