// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

resource "pve_acl" "example" {
  path      = "/vms/100"
  role      = "PVEVMUser"
  type      = "user"
  user_id   = "ops@pam"
  propagate = true
}
