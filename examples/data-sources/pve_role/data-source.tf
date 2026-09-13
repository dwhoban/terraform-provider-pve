# Built-in roles resolve through the same data source.
data "pve_role" "pvevmadmin" {
  roleid = "PVEVMAdmin"
}

output "pvevmadmin_privileges" {
  value = [for p in data.pve_role.pvevmadmin.privs : p]
}
