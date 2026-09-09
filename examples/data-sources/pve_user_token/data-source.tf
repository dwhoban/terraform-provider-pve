# Copyright (c) HashiCorp, Inc.

# List every API token of a PVE user.
data "pve_user_token" "root_tokens" {
  userid = "root@pam"
}

# Or read one token's settings.
data "pve_user_token" "ci" {
  userid  = "root@pam"
  tokenid = "ci"
}

output "ci_token_privsep" {
  value = data.pve_user_token.ci.privsep
}
