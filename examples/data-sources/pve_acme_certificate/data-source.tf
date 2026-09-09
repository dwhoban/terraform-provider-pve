# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_acme_certificate" "pve1" {
  node = "pve1"
}

output "pve1_acme_domains" {
  value = data.pve_acme_certificate.pve1.domains
}

output "pve1_acme_not_after" {
  value = data.pve_acme_certificate.pve1.not_after
}
