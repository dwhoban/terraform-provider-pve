# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_realm_ldap" "corp" {
  realm = "corp"
}

output "ldap_base_dn" {
  value = data.pve_realm_ldap.corp.base_dn
}
