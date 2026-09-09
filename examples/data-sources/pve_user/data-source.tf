# Copyright (c) HashiCorp, Inc.

# Look up a PVE user account.
data "pve_user" "root" {
  userid = "root@pam"
}

output "root_email" {
  value = data.pve_user.root.email
}
