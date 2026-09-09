# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_ceph_mon" "pve1" {
  node  = "pve1"
  monid = "pve1"
}

output "mon_state" {
  value = data.pve_ceph_mon.pve1.state
}
