# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Prerequisite: an ACME account must exist in the cluster configuration
# (see pve_acme_account); DNS challenges additionally need a DNS plugin
# (see pve_acme_dns_plugin).
resource "pve_acme_certificate" "le" {
  node    = "pve1"
  domains = ["pve1.example.com"]
}

output "pve1_acme_certificate_fingerprint" {
  value = resource.pve_acme_certificate.le.fingerprint
}
