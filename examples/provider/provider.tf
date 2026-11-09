provider "pve" {
  # Proxmox VE API endpoint; may also be set via PVE_API_URL.
  api_url = "https://pve.example.com:8006"

  # API token auth (recommended); may also be set via PVE_API_TOKEN.
  # api_token = "root@pam!terraform=00000000-0000-0000-0000-000000000000"

  # Alternatively username/password (+ totp) auth; PVE_USER / PVE_PASSWORD / PVE_TOTP.
  # username = "root@pam"
  # password = "secret"

  # Accept self-signed certificates (PVE_INSECURE), or pin one with ca_cert (PVE_CA_CERT).
  # insecure = true
}
