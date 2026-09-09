# Copyright (c) HashiCorp, Inc.

data "pve_realm_sync_job" "corp_nightly" {
  id = "corp-nightly"
}

output "next_scheduled_sync" {
  value = data.pve_realm_sync_job.corp_nightly.next_run
}
