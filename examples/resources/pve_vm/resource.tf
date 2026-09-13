# A minimal QEMU guest on node pve1. The provider allocates the next free
# VMID at create time because `vmid` is omitted.
resource "pve_vm" "web" {
  node     = "pve1"
  name     = "web01"
  ostype   = "l26"
  cores    = 2
  sockets  = 1
  memory   = 2048
  cpu_type = "host"
  scsihw   = "virtio-scsi-single"
  onboot   = true
  tags     = ["prod", "web"]

  disks = [
    {
      id       = "scsi0"
      storage  = "local-lvm"
      size     = "32G"
      iothread = true
      discard  = "on"
      ssd      = true
    },
  ]

  network_interfaces = [
    {
      id       = "net0"
      model    = "virtio"
      bridge   = "vmbr0"
      vlan_tag = 10
      firewall = true
    },
  ]

  cloud_init = {
    user         = "admin"
    searchdomain = "example.internal"
    nameserver   = "10.0.0.1"
    # Read from a public key file on the machine running Terraform:
    # sshkeys = file("/home/user/.ssh/id_ed25519.pub")
    sshkeys = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExamplePublicKeyPlaceholder"

    ipconfig = {
      ipconfig0 = "ip=dhcp"
    }
  }
}

# A guest created by cloning a template on the same node.
resource "pve_vm" "cloned" {
  node    = "pve1"
  vmid    = 105
  cores   = 4
  memory  = 4096
  started = true

  clone = {
    source_vmid = 9000
    full        = true
    storage     = "local-lvm"
    format      = "qcow2"
    name        = "clone01"
  }
}
