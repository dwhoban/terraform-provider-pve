# Copyright (c) HashiCorp, Inc.

data "pve_node" "pve1" {
  node = "pve1"
}

output "pve1_pve_version" {
  value = data.pve_node.pve1.pve_version.version
}

output "pve1_certificate_fingerprints" {
  value = [for cert in data.pve_node.pve1.certificates : cert.fingerprint]
}
