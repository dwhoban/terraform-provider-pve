resource "pve_acme_account" "default" {
  name    = "default"
  contact = ["mailto:ops@example.com"]
  tos_url = "https://acme-v02.api.letsencrypt.org/directory"
}
