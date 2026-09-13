data "pve_realms" "all" {}

output "authentication_realms" {
  value = [for r in data.pve_realms.all.realms : "${r.realm} (${r.type})"]
}
