---
page_title: "Node networking"
subcategory: ""
description: |-
  Pending network changes, committing them per node with pve_node_network_apply, opting single interfaces into apply_on_change, and rolling back.
---

# Node networking

## Pending changes

PVE writes interface changes as pending: `pve_node_network` create, update, and delete edit the node's network configuration, but nothing is active until the changes are applied. An apply (`PUT /nodes/{node}/network`) commits every pending change on the node and reloads its network configuration. Reads return the pending configuration, so Terraform state stays consistent with what you declared before the apply runs.

## One apply per node (recommended)

Declare the `pve_node_network_apply` action once per node and trigger it after all interfaces of that node are written. Use one `terraform_data` per node whose `input` lists every `pve_node_network` of that node, so any change triggers a single apply after all interfaces are written.

```terraform
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
```

## Per-interface apply

Set `apply_on_change = true` on a `pve_node_network` to apply after that interface's own create or update. Each changed interface causes one reload, and the apply commits other pending changes on the node too. `apply_on_change` is a provider-side setting and is never sent to PVE.

## Removals and rollback

Destroying a `pve_node_network` only records a pending removal, and Terraform has no destroy trigger. Commit it with `terraform apply -invoke=action.pve_node_network_apply.<label>` (Terraform ≥ 1.14, see the [`terraform apply` reference](https://developer.hashicorp.com/terraform/cli/commands/apply)).

Discard pending changes with `terraform apply -invoke=action.pve_node_network_rollback.<label>` after declaring that action block.
