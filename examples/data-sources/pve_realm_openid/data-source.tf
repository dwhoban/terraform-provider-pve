# Copyright (c) HashiCorp, Inc.

data "pve_realm_openid" "keycloak" {
  realm = "keycloak"
}

output "openid_issuer" {
  value = data.pve_realm_openid.keycloak.issuer_url
}
