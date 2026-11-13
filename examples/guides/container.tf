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
