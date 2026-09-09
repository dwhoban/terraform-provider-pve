# Copyright (c) HashiCorp, Inc.

# Credentials must be base64 encoded before passing them to `data`. For a
# PowerDNS provider the file holds `pdns_api_url=...` and `pdns_api_key=...`
# lines; build the value with:
#   data = filebase64("${path.module}/pdns-creds.txt")
resource "pve_acme_dns_plugin" "pdns" {
  plugin = "pdns"
  api    = "pdns"
  data   = "cGRuc19hcGlfdXJsPWh0dHBzOi8vcGRucy5leGFtcGxlLmNvbQpWRVJBc2VydmVyX0FQSV9LRVk9ZXhhbXBsZS1rZXkK"
  nodes  = ["pve1"]
}
