# Firewall options of the SDN vnet1. One instance per vnet manages the
# listed options; removing an attribute clears it via the `delete`
# parameter. Destroy only forgets the state.
resource "pve_sdn_firewall_options" "vnet1" {
  vnet   = "vnet1"
  enable = true

  policy_forward    = "DROP"
  log_level_forward = "debug"
}
