# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

data "pve_ceph_pool" "rbd" {
  node = "pve1"
  name = "rbd"
}

output "rbd_pool_id" {
  value = data.pve_ceph_pool.rbd.pool_id
}

output "rbd_pg_num" {
  value = data.pve_ceph_pool.rbd.pg_num
}
