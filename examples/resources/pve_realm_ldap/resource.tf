# Copyright (c) HashiCorp, Inc.

resource "pve_realm_ldap" "corp" {
  realm        = "corp"
  comment      = "Corporate LDAP"
  server1      = "ldap.example.com"
  server2      = "ldap2.example.com"
  port         = 636
  mode         = "ldaps"
  verify       = true
  base_dn      = "dc=example,dc=com"
  password     = "s3cret-bind-password" # stored by PVE in /etc/pve/priv/realm/corp.pw
  user_attr    = "uid"
  user_classes = "inetorgperson, posixaccount"
  default      = false
}
