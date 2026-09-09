# Copyright (c) HashiCorp, Inc.
# Credentials must be base64 encoded before passing them to `data`.

resource "pve_acme_dns_plugin" "pdns" {
  plugin = "pdns"
  api    = "pdns"
  data   = filebase64("${path.module}/pdns-creds.txt")
  nodes  = ["pve1"]
}
