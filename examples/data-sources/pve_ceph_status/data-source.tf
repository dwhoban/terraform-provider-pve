# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_ceph_status" "pve1" {
  node = "pve1"
}

output "ceph_health" {
  value = data.pve_ceph_status.pve1.health
}

output "ceph_quorum" {
  value = data.pve_ceph_status.pve1.quorum_names
}
