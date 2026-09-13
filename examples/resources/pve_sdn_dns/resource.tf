# A reverse-DNS plugin registers guest IPs with PowerDNS. Run the
# `pve_sdn_apply` action afterwards to activate pending SDN changes.

resource "pve_sdn_dns" "pdns1" {
  dns           = "pdns1"
  url           = "https://pdns.example:8081/api/v1"
  key           = var.pdns_api_key
  reversemaskv6 = 64
  ttl           = 60
}

variable "pdns_api_key" {
  type      = string
  sensitive = true
}
