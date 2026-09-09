# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_realm_ad" "ad" {
  realm = "ad"
}

output "ad_domain" {
  value = data.pve_realm_ad.ad.domain
}
