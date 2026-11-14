---
page_title: "When is SSH required?"
subcategory: ""
description: |-
  The provider is API-first. SSH is optional and used only for operations the Proxmox VE API cannot perform.
---

# When is SSH required?

The provider talks to Proxmox VE through its REST API. **Most configurations never need SSH**, and without an `ssh` block the provider never opens an SSH connection.

SSH is used only for:

| Feature | Why SSH is needed |
|---|---|
| `pve_ceph_install` action | The API has no endpoint that installs packages; the action runs `/usr/sbin/pveceph install` on the node. |
| `pve_cluster_qdevice` resource | The API cannot configure a QDevice; the resource runs `/usr/bin/pvecm qdevice setup` and `/usr/bin/pvecm qdevice remove` on a cluster member. |
| `ssh.root_ticket = true` | Proxmox VE restricts some API calls (container bind mounts, privileged-container feature flags, `arch`, PCI/USB passthrough without a mapping, …) to a logged-in `root@pam`. With this opt-in the provider mints a short-lived `root@pam` ticket on the node over SSH, so an API token can still perform them. |

Everything else — VMs, containers, storage, networking, pools, RBAC, firewall, SDN, backup, HA, Ceph pools/OSDs/monitors, and so on — uses the API only.

## Configuration

```terraform
provider "pve" {
  api_url   = "https://10.0.0.2:8006"
  api_token = "root@pam!terraform=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

  ssh {
    # username defaults to the PAM user of the API credentials; it is
    # required with a non-PAM token.
    username = "root"
    agent    = true # or private_key = file("~/.ssh/id_ed25519") / password

    # Optional: pin a node's SSH address (otherwise the provider uses the
    # node's address from the API, then its name as a DNS name).
    node {
      name    = "pve1"
      address = "10.0.0.2"
    }
  }
}
```

Every `ssh` setting can also come from an environment variable: `PVE_SSH_USERNAME`, `PVE_SSH_PASSWORD`, `PVE_SSH_PRIVATE_KEY`, `PVE_SSH_AGENT`, `PVE_SSH_AUTH_SOCK`, `PVE_SSH_KNOWN_HOSTS`, `PVE_SSH_INSECURE_IGNORE_HOST_KEY`, `PVE_SSH_ROOT_TICKET`.

- **Authentication**: SSH agent, then private key (unencrypted PEM/OpenSSH), then password. With username/password API login of a PAM user, the same password is reused for SSH.
- **Host keys are verified** against `~/.ssh/known_hosts` (or `known_hosts_file`). Add the node's key first (`ssh-keyscan -t ed25519 <node> >> ~/.ssh/known_hosts`). `insecure_ignore_host_key = true` disables verification and is **not recommended**.
- **Connections are lazy**: nothing connects until an SSH-backed operation runs, and nothing is written to disk.
- Commands are passed as argument vectors and every argument is single-quoted, so a strict sudoers rule matches the exact command line.

## Non-root SSH user (sudo)

If `ssh.username` is not `root`, commands run through `sudo -n` and the user needs passwordless sudo for **exactly** the commands you use. Do **not** grant sudo on `pveceph`, `pvecm` or `perl` wholesale — grant the pattern below, which sudo (1.9.10+) matches as a regular expression against the full argument list:

```text
# /etc/sudoers.d/terraform  (edit with: visudo -f /etc/sudoers.d/terraform)
terraform ALL=(root) NOPASSWD: /usr/sbin/pveceph ^install --repository (enterprise|no-subscription|test)( --version [a-z0-9][a-z0-9.-]*)?( --allow-experimental)?$
terraform ALL=(root) NOPASSWD: /usr/bin/pvecm ^qdevice (setup [0-9A-Fa-f.:]+( --force)?( --network [0-9A-Fa-f.:]+/[0-9]{1,3})?|remove)$
```

~> `root_ticket` requires `username = "root"`: minting a `root@pam` ticket needs a root shell, and `sudo` on `perl` would be root-equivalent. The provider refuses to start with `root_ticket` and a non-root SSH user.

## root_ticket

```terraform
provider "pve" {
  api_url   = "https://10.0.0.2:8006"
  api_token = "root@pam!terraform=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

  ssh {
    agent       = true
    username    = "root"
    root_ticket = true
  }
}
```

While `root_ticket` is on, every call under `/nodes/{node}/` is authorized and logged as `root@pam`, so the token's own permissions no longer limit those calls. The root-only cluster configuration writes (`POST /cluster/config`, `POST /cluster/config/join`, `POST`/`DELETE /cluster/config/nodes/{node}`, used by `pve_cluster`, `pve_cluster_join`, and `pve_cluster_node_remove`) are authorized the same way, with a ticket minted on the API endpoint's node (resolved through `GET /cluster/status`, which needs `Sys.Audit` on `/`). Every other call outside `/nodes/{node}/` (for example `/access`) keeps using the configured credentials. Tickets are minted per node, cached for an hour, and never logged.

## Installing Ceph

```terraform
action "pve_ceph_install" "pve1" {
  config {
    node       = "pve1"
    repository = "no-subscription"
  }
}
```

The action is idempotent: it probes the node's Ceph API first and does nothing if Ceph is already installed. Run it before `pve_ceph_init`, for example with an `action_trigger` on the resource that precedes your Ceph setup.
