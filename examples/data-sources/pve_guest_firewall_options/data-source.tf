# Firewall options of the lxc container 101 on node pve1.
data "pve_guest_firewall_options" "ct101" {
  node       = "pve1"
  guest_type = "lxc"
  vmid       = 101
}
