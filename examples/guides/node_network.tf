locals {
  node = "pve1"
}

# Both interfaces are written as pending changes; nothing is active yet.
resource "pve_node_network" "vmbr10" {
  node              = local.node
  iface             = "vmbr10"
  type              = "bridge"
  bridge_vlan_aware = true
  autostart         = true
}

resource "pve_node_network" "vmbr20" {
  node      = local.node
  iface     = "vmbr20"
  type      = "bridge"
  cidr      = "192.0.2.2/24" # TEST-NET-1: never routed
  autostart = true
}

# Commits every pending change on the node in a single reload.
action "pve_node_network_apply" "pve1" {
  config {
    node = local.node
  }
}

# Discards every pending change on the node. Run on demand with
# terraform apply -invoke=action.pve_node_network_rollback.pve1
action "pve_node_network_rollback" "pve1" {
  config {
    node = local.node
  }
}

# One apply per node, after every interface of the node is written: the
# input lists them all, so any change to any of them triggers one apply.
resource "terraform_data" "pve1_network" {
  input = [pve_node_network.vmbr10, pve_node_network.vmbr20]

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.pve_node_network_apply.pve1]
    }
  }
}
