# Copyright (c) HashiCorp, Inc.

resource "pve_group" "admins" {
  groupid = "admins"
  comment = "Cluster administrators"

  # members is read-only: PVE manages group membership through each user.
  # Grant membership with the `groups` attribute of a pve_user resource.
}

output "admins_members" {
  value = [for u in resource.pve_group.admins.members : u]
}
