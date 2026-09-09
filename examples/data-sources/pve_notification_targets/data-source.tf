# Copyright (c) HashiCorp, Inc.

data "pve_notification_targets" "all" {}

output "notification_target_names" {
  value = [for t in data.pve_notification_targets.all.targets : t.name]
}
