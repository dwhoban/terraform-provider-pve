# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_appliances" "pve1" {
  node = "pve1"
}

output "appliance_templates" {
  value = [for a in data.pve_appliances.pve1.appliances : a.template]
}
