---
page_title: "Cluster lifecycle"
subcategory: ""
description: |-
  Creating a Proxmox VE cluster with pve_cluster, joining peers with pve_cluster_join, and observing it with the cluster data sources.
---

# Cluster lifecycle

Cluster creation and joining are **root-only** operations. Use `root@pam` password credentials, or an API token together with [`ssh.root_ticket`](ssh.md): the provider then mints a `root@pam` ticket over SSH on the node the API endpoint belongs to (found through `GET /cluster/status`, so the token needs `Sys.Audit` on `/`). Joining a peer still needs the seed's root password as the `password` argument, because the API requires it.

1. **Create** the cluster on the seed node with `pve_cluster`. The resource is create-only: every attribute forces replacement, and destroy removes the node from state only (PVE has no cluster-delete API — leaving a cluster is a manual operation).
2. **Join** each peer node with the `pve_cluster_join` action, run in a separate root module against that node's root context. It needs the seed's address, TLS fingerprint, and root password — the `pve_cluster_join_info` data source exposes the fingerprint, allowed IPs, and totem configuration.
3. **Remove** a node with the `pve_cluster_node_remove` action (run on any remaining member).

```terraform
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
```

Corosync links are the `link` block: `link[0]` is `link0`, up to `link7`. The link `address` is the value PVE writes without a key (the default property of the property string); `priority` is optional.

For observation, the provider ships cluster data sources: `pve_cluster` (status and options), `pve_cluster_nodes`, `pve_cluster_join_info`, `pve_cluster_api_version`, `pve_cluster_jobs`, `pve_cluster_qdevice`, `pve_cluster_totem`, and `pve_cluster_mappings`.

## QDevice

`pve_cluster_qdevice` sets up an external corosync QDevice (a vote-only tie-breaker, usually for two-node clusters). The Proxmox VE API can only *read* QDevice status, so this resource runs `pvecm qdevice setup` / `pvecm qdevice remove` on a cluster member over the provider's `ssh` block (see [When is SSH required?](ssh.md)).

```terraform
resource "pve_cluster_qdevice" "tiebreaker" {
  node    = "pve1"
  address = "10.0.0.50" # IP of the host running corosync-qnetd
}
```

Prerequisites, all outside the provider's control:

- `corosync-qnetd` is installed on the QNet host and `corosync-qdevice` on **every** cluster node.
- All nodes are online.
- `pvecm` copies keys and runs BatchMode `ssh`/`scp` from the node to the QNet host as root: the node's `/root/.ssh/id_rsa.pub` must already be in the QNet host's root `authorized_keys`, and the QNet host's key must be in the node's `known_hosts`.
- Set `force = true` for clusters with an odd node count.

Destroy runs `pvecm qdevice remove`. Import with `terraform import pve_cluster_qdevice.tiebreaker pve1/10.0.0.50` (`node/address`).
