# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Built-in roles resolve through the same data source.
data "pve_role" "pvevmadmin" {
  roleid = "PVEVMAdmin"
}

output "pvevmadmin_privileges" {
  value = [for p in data.pve_role.pvevmadmin.privs : p]
}
