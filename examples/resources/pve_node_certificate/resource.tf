# Copyright (c) HashiCorp, Inc.

# Replace the PEM placeholders with your certificate material, or load it
# from files: certificates_pem = file("certs/pve1-fullchain.pem").
resource "pve_node_certificate" "custom" {
  node             = "pve1"
  certificates_pem = <<-EOT
    -----BEGIN CERTIFICATE-----
    MIIB...your full chain (leaf first, then intermediates)...
    -----END CERTIFICATE-----
  EOT
  private_key      = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MIIB...your private key...
    -----END PRIVATE KEY-----
  EOT
  restart          = true
}

output "pve1_certificate_fingerprint" {
  value = pve_node_certificate.custom.fingerprint
}
