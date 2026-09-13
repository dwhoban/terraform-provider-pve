data "pve_realm_ldap" "corp" {
  realm = "corp"
}

output "ldap_base_dn" {
  value = data.pve_realm_ldap.corp.base_dn
}
