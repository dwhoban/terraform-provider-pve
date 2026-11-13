---
page_title: "Virtual machines"
subcategory: ""
description: |-
  Creating and operating QEMU VMs with pve_vm: disks, NICs with cloud-init addresses, clone-at-create, import, and lifecycle actions.
---

# Virtual machines

`pve_vm` manages the full QEMU configuration lifecycle. Two attributes are required — `node` and `vmid`; allocate a free id inline with the provider function `provider::pve::nextid()`.

Disks and NICs are **slot-family blocks**, not flat string attributes. Each `disk` element takes one PVE slot (`interface`, e.g. `scsi0`); each `network` element one NIC slot (`network[0]` is `net0`). New volumes are allocated from `storage` + `size_gb`; the allocated `volume_id` and assigned `mac_address` are computed and read back into state.

```terraform
resource "pve_vm" "web" {
  node   = "pve1"
  vmid   = provider::pve::nextid()
  name   = "web"
  ostype = "l26"
  cores  = 2
  memory = 2048
  onboot = true

  disk {
    interface = "scsi0"
    storage   = "local-lvm"
    size_gb   = 32
  }

  network {
    model   = "virtio"
    bridge  = "vmbr0"
    ipv4    = "10.0.0.10/24"
    gateway = "10.0.0.1"
  }

  cdrom {
    iso = "local:iso/debian-12.11.0-amd64-netinst.iso"
  }
}

# The VM is created stopped; start it once it exists.
resource "terraform_data" "web_start" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_start.web]
    }
  }
}

action "pve_vm_start" "web" {
  config {
    node = pve_vm.web.node
    vmid = pve_vm.web.vmid
  }
}

# Growing a disk is not a config edit: storage/size_gb are create-only, so
# the dedicated action performs the resize on the running guest.
resource "terraform_data" "web_grow" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_disk_resize.web]
    }
  }
}

action "pve_vm_disk_resize" "web" {
  config {
    node = pve_vm.web.node
    vmid = pve_vm.web.vmid
    disk = "scsi0"
    size = 64
  }
}
```

Notes:

- **Cloud-init addresses** (`ipv4`/`gateway`/`ipv6`/`gateway6` on a `network` element) land on the matching `ipconfig<n>` slot. They require a cloud-init setup on the guest (e.g. `ciuser`, `sshkeys`, or a cloud-init disk).
- **Resizing and moving disks are not config edits**: `storage` and `size_gb` are create-only, and the plan rejects a change with a pointer to the right action — `pve_vm_disk_resize`, `pve_vm_disk_move` (and `pve_vm_disk_unlink`).
- **Removing a block element unsets the slot** via PVE's canonical `delete` parameter; reordering a list keeps slots stable because elements are matched by wire key, not position.
- **Clone-at-create**: set `clone_source_vmid` and the VM is created from that source via the clone endpoint instead of a fresh install.
- **Import** an existing guest with its identity: `terraform import pve_vm.web pve1/101` (the id format is `node/vmid`). Import reads back the server's slots wholesale.
- **Lifecycle** (start, stop, reboot, shutdown, suspend/resume, migrate, remote-migrate, snapshot rollback, convert to template, fstrim) is expressed with actions wired to lifecycle events via `action_trigger` — see the example above. `pve_vm_migrate` moves the guest to another `target` node.

The full argument reference is on the [pve_vm resource page](../resources/vm.md); inventory and runtime facts (status, agent info, pending changes) are on the `pve_vm` data source.
