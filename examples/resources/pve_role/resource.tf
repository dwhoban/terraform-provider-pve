# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_role" "operator" {
  roleid = "operator"
  privs = [
    "VM.Audit",
    "VM.Console",
    "VM.PowerMgmt",
    "VM.Migrate",
  ]
}
