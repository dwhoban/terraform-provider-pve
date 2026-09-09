// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

data "pve_permissions" "ops" {
  userid = "ops@pam"
}

output "ops_permissions" {
  value = data.pve_permissions.ops.entries
}
