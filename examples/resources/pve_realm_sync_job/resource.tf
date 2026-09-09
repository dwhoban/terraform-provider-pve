# Copyright (c) HashiCorp, Inc.

# Nightly sync job for the corporate LDAP realm.
resource "pve_realm_sync_job" "corp_nightly" {
  id              = "corp-nightly"
  realm           = "corp"
  schedule        = "mon..fri 02:30"
  scope           = "both"
  remove_vanished = "entry;acl"
  enable_new      = true
  enabled         = true
  comment         = "Nightly LDAP sync"
}
