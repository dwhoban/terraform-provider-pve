# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_ceph_osd" "osd0" {
  node   = "pve1"
  osd_id = 0
}

output "osd0_objectstore" {
  value = data.pve_ceph_osd.osd0.osd_objectstore
}
