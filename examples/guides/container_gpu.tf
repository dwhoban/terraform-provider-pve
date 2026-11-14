# Passing an Intel Arc GPU into an unprivileged container.
# Find the gids with `getent group render video` inside the container.
resource "pve_container" "media" {
  node         = "pve1"
  vmid         = 210
  hostname     = "media"
  ostemplate   = "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  unprivileged = true
  password     = "CHANGE_ME-root-password"

  rootfs {
    storage = "local-lvm"
    size_gb = 16
  }

  network {
    name   = "eth0"
    bridge = "vmbr0"
    ipv4   = "dhcp"
  }

  device {
    path = "/dev/dri/renderD128"
    gid  = 104 # `render` group inside the container
    mode = "0660"
  }

  device {
    path = "/dev/dri/card0"
    gid  = 44 # `video` group inside the container
    mode = "0660"
  }
}
