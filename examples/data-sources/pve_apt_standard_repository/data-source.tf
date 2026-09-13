# Look up one standard repository on node pve1 by its handle. `status` is
# null while the repository is not configured.
data "pve_apt_standard_repository" "enterprise" {
  node   = "pve1"
  handle = "enterprise"
}

output "pve1_enterprise_status" {
  value = data.pve_apt_standard_repository.enterprise.status
}
