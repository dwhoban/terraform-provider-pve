---
page_title: "Storages and files"
subcategory: ""
description: |-
  Managing storages with pve_storage, moving ISO images and templates onto them with the file actions, and inventorying existing storages.
---

# Storages and files

`pve_storage` manages a storage definition: `storage` (its id) and `type` are required — `dir`, `nfs`, `cifs`, `lvm`, `lvmthin`, `zfs`, `zfspool`, `btrfs`, `iscsi`, `iscsidirect`, `rbd`, `cephfs`, `pbs`, or `esxi`. Type-specific fields (`path`, `server`, `export`, `pool`, …), served `content` kinds, and node restrictions (`nodes`) round out the definition.

Files (ISO images, container templates, backups) are moved onto storages with actions rather than a resource:

- `pve_file_download_url` — the node downloads from `url` directly.
- `pve_file_upload` — upload from the machine running Terraform.

Both target `node` + `storage` + `filename` and declare the `content` kind (`iso`, `vztmpl`, or `import`); both accept a `checksum` + `checksum_algorithm` pair. The `pve_file` resource manages the files PVE knows about, including deleting them again.

```terraform
# A directory storage on one node, serving ISO images and container
# templates.
resource "pve_storage" "iso" {
  storage = "iso-store"
  type    = "dir"
  path    = "/srv/iso"
  content = ["iso", "vztmpl"]
  nodes   = ["pve1"]
}

# Download an ISO onto it. The action runs when the storage exists.
resource "terraform_data" "iso_download" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_file_download_url.debian]
    }
  }
}

action "pve_file_download_url" "debian" {
  config {
    node     = "pve1"
    storage  = pve_storage.iso.storage
    filename = "debian-12.11.0-amd64-netinst.iso"
    url      = "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.11.0-amd64-netinst.iso"
    content  = "iso"
  }
}

# Inventory: what is already on a node's storages.
data "pve_node_storages" "pve1" {
  node = "pve1"
}
```

To see what already exists, use the inventory data sources: `pve_node_storages` (per node), `pve_storage` (one storage's configuration and usage), `pve_storage_scan_*` (NFS/CIFS/iSCSI/LVM/LVMthin/PBS/ZFS discovery scans), and `pve_storage_identity`.
