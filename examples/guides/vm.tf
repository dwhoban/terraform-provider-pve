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
