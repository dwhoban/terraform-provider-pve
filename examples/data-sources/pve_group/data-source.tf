# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_group" "admins" {
  groupid = "admins"
}

output "admin_user_ids" {
  value = [for u in data.pve_group.admins.members : u]
}
