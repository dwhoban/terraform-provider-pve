# Copyright (c) HashiCorp, Inc.

data "pve_appliances" "pve1" {
  node = "pve1"
}

output "appliance_templates" {
  value = [for a in data.pve_appliances.pve1.appliances : a.template]
}
