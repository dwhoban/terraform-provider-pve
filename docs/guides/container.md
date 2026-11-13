---
page_title: "Containers"
subcategory: ""
description: |-
  LXC containers with pve_container: templates, rootfs and mount points, NICs, clone-at-create, and lifecycle actions.
---

# Containers

`pve_container` manages LXC guests. Three attributes are required — `node`, `vmid`, and `ostemplate` (a template volume, e.g. `local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst`; upload one with the `pve_file_download_url` action, see the [storage guide](storage.md)).

Storage and NICs are **slot-family blocks**:

- `rootfs` is a single (fixed) block: allocate from `storage` + `size_gb`; PVE normalizes the allocation to a concrete `volume_id`, which is read back computed.
- `mount_point` elements map to `mp0`, `mp1`, … by list position; bind mounts need `root@pam` rights (see the [SSH guide](ssh.md) on `ssh.root_ticket`).
- `network` elements map to `net0`, `net1`, … by list position; `name` is the interface name inside the container (e.g. `eth0`).

```terraform
resource "pve_container" "app" {
  node         = "pve1"
  vmid         = 200
  hostname     = "app"
  ostemplate   = "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  unprivileged = true
  password     = "CHANGE_ME-root-password"
  memory       = 1024
  onboot       = true

  rootfs {
    storage = "local-lvm"
    size_gb = 8
  }

  network {
    name    = "eth0"
    bridge  = "vmbr0"
    ipv4    = "10.0.0.20/24"
    gateway = "10.0.0.1"
  }
}

# Containers are created stopped; start and keep them running.
resource "terraform_data" "app_start" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_container_start.app]
    }
  }
}

action "pve_container_start" "app" {
  config {
    node = pve_container.app.node
    vmid = pve_container.app.vmid
  }
}
```

Notes:

- **Clone-at-create**: set `clone_source_vmid` to clone an existing container instead of extracting the template.
- **Resizing** rootfs/mount points is action-based: `pve_container_resize`; `pve_container_move_volume` moves a volume to another storage.
- **Lifecycle** (start, stop, reboot, shutdown, suspend/resume, migrate, remote-migrate, convert to template, snapshot rollback) is expressed with actions wired via `action_trigger` — see the example.
- **Import** an existing container: `terraform import pve_container.app pve1/200` (`node/vmid`).

The full argument reference is on the [pve_container resource page](../resources/container.md); the `pve_container` data source exposes configuration, pending changes, and runtime state.
