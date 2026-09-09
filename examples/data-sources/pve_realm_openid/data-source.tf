# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_realm_openid" "keycloak" {
  realm = "keycloak"
}

output "openid_issuer" {
  value = data.pve_realm_openid.keycloak.issuer_url
}
