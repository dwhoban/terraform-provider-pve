# Copyright (c) HashiCorp, Inc.

# Preview (dry run) what a full sync of the corporate LDAP realm would
# change, removing vanished entries and their ACLs. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_realm_sync.preview]
action "pve_realm_sync" "preview" {
  config {
    realm           = "corp"
    scope           = "both"
    dry_run         = true
    enable_new      = true
    remove_vanished = "entry;acl"
  }
}
