resource "pve_guest_firewall_rules" "vm100" {
  node       = "pve1"
  guest_type = "qemu"
  vmid       = 100
  rules = [
    {
      type    = "in"
      action  = "ACCEPT"
      proto   = "tcp"
      dport   = "443"
      iface   = "net0"
      comment = "HTTPS"
    },
    {
      type    = "out"
      action  = "ACCEPT"
      comment = "Allow all outbound"
    },
  ]
}
