// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

data "pve_acl" "all" {}

output "acl_entries" {
  value = data.pve_acl.all.entries
}
