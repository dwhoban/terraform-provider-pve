data "pve_version" "current" {}

output "pve_version" {
  value = data.pve_version.current.version
}
