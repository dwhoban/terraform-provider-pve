# Copyright (c) HashiCorp, Inc.

data "pve_node_certificate" "pve1" {
  node = "pve1"
}

output "pve1_web_certificate_fingerprint" {
  value = [for cert in data.pve_node_certificate.pve1.certificates : cert.fingerprint if cert.filename == "pveproxy-ssl.pem"]
}

output "pve1_web_certificate_not_after" {
  value = [for cert in data.pve_node_certificate.pve1.certificates : cert.not_after if cert.filename == "pveproxy-ssl.pem"]
}
