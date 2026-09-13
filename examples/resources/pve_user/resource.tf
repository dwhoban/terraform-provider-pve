# Manage a PVE user account in the `pve` realm.
resource "pve_user" "ci" {
  userid    = "ci@pve"
  comment   = "CI automation account"
  email     = "ci@example.com"
  firstname = "CI"
  lastname  = "Bot"
  enable    = true
  expire    = 0

  groups = ["ops"]

  # Only sent at create; PVE returns the secret-free response and never
  # allows reading the password back.
  password = var.ci_password
}

variable "ci_password" {
  type      = string
  sensitive = true
}
