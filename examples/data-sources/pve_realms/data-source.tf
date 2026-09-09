# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_realms" "all" {}

output "authentication_realms" {
  value = [for r in data.pve_realms.all.realms : "${r.realm} (${r.type})"]
}
