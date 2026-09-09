# Copyright (c) HashiCorp, Inc.

# LXC container created from an OS template on local storage.
resource "pve_container" "ct1" {
  vmid         = 100
  node         = "pve1"
  ostemplate   = "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  hostname     = "ct1"
  unprivileged = true
  cores        = 2
  memory       = 512

  mount_points = [
    {
      id      = "rootfs"
      storage = "local-lvm"
      size    = "8G"
    },
    {
      id         = "mp0"
      mountpoint = "/data"
      storage    = "local-lvm"
      size       = "4G"
      backup     = true
    },
  ]
}

# LXC container cloned from an existing container (create-only clone block).
resource "pve_container" "ct2" {
  vmid = 101
  node = "pve1"

  clone = {
    source_vmid = 100
    hostname    = "ct2"
    full        = true
    storage     = "local-lvm"
  }
}
