# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_realm_ad" "ad" {
  realm          = "ad"
  comment        = "Active Directory"
  domain         = "ad.example.com"
  server1        = "dc1.ad.example.com"
  server2        = "dc2.ad.example.com"
  port           = 636
  mode           = "ldaps"
  case_sensitive = false
  default        = false
}
