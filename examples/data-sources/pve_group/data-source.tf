data "pve_group" "admins" {
  groupid = "admins"
}

output "admin_user_ids" {
  value = [for u in data.pve_group.admins.members : u]
}
