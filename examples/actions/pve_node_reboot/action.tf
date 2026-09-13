# Destructive: reboots node pve1 immediately. Guests running on pve1 are
# interrupted unless HA relocates them. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_reboot.reboot_pve1]
action "pve_node_reboot" "reboot_pve1" {
  config {
    node = "pve1"
  }
}
