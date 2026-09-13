# Enable the no-subscription PVE repository on node pve1. Destroying this
# resource only forgets it from state: the PVE API cannot remove repository
# entries.
resource "pve_apt_standard_repository" "no_subscription" {
  node   = "pve1"
  handle = "no-subscription"
}
