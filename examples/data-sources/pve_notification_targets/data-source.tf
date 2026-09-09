# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_notification_targets" "all" {}

output "notification_target_names" {
  value = [for t in data.pve_notification_targets.all.targets : t.name]
}
