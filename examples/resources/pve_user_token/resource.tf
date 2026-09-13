# Manage the CI user and an API token for it. The token secret is returned
# exactly once, at create, and stored in the state as `token_value`.
resource "pve_user" "ci" {
  userid  = "ci@pve"
  comment = "CI automation account"
  groups  = ["operators"]
}

resource "pve_user_token" "ci" {
  userid  = pve_user.ci.userid
  tokenid = "ci"
  comment = "Token for the CI pipeline"
  privsep = true
  expire  = 0
}

output "ci_token" {
  value     = pve_user_token.ci.token_value
  sensitive = true
}
