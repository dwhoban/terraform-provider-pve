# Copyright (c) HashiCorp, Inc.

resource "pve_realm_openid" "keycloak" {
  realm      = "keycloak"
  comment    = "Keycloak SSO"
  issuer_url = "https://sso.example.com/realms/pve"
  client_id  = "proxmox"
  client_key = "s3cret-client-key"
  scopes     = "email profile"
  autocreate = true
  default    = false
}
