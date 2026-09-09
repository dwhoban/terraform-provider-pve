# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node_certificate" "custom" {
  node             = "pve1"
  certificates_pem = file("certs/pve1-fullchain.pem")
  private_key      = file("certs/pve1.key")
  restart          = true
}

output "pve1_certificate_fingerprint" {
  value = resource.pve_node_certificate.custom.fingerprint
}
