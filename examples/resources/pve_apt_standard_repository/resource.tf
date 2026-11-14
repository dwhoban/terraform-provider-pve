resource "pve_apt_standard_repository" "no_subscription" {
  node   = "pve1"
  handle = "no-subscription"
}

resource "pve_apt_standard_repository" "enterprise" {
  node    = "pve1"
  handle  = "enterprise"
  enabled = false
}
