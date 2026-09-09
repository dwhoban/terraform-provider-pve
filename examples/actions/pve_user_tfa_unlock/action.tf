# Copyright (c) HashiCorp, Inc.

# Security-sensitive: clears the TFA lockout so the operator can
# authenticate and re-enroll second factors. Validate provenance of any
# configuration invoking this.
action "pve_user_tfa_unlock" "unlock_operator" {
  config {
    userid = "ops@pam"
  }
}
