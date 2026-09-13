data "pve_realm_ad" "ad" {
  realm = "ad"
}

output "ad_domain" {
  value = data.pve_realm_ad.ad.domain
}
