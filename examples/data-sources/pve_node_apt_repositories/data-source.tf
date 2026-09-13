# Parsed APT repository configuration of node pve1.
data "pve_node_apt_repositories" "pve1" {
  node = "pve1"
}

# Optionally fetch a package changelog alongside the repository state.
# data "pve_node_apt_repositories" "pve1_changelog" {
#   node    = "pve1"
#   package = "pve-manager"
# }

output "pve1_standard_repos" {
  value = { for repo in data.pve_node_apt_repositories.pve1.standard_repositories : repo.handle => repo.status }
}

output "pve1_repository_rows" {
  value = [for repo in data.pve_node_apt_repositories.pve1.repositories : "${repo.path}[${repo.index}] enabled=${repo.enabled}"]
}

output "pve1_apt_file_errors" {
  value = [for err in data.pve_node_apt_repositories.pve1.errors : "${err.path}: ${err.error}"]
}

# output "pve1_pve_manager_changelog" {
#   value = data.pve_node_apt_repositories.pve1_changelog.changelog
# }
