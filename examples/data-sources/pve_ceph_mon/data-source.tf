# Copyright (c) HashiCorp, Inc.

data "pve_ceph_mon" "pve1" {
  node  = "pve1"
  monid = "pve1"
}

output "mon_state" {
  value = data.pve_ceph_mon.pve1.state
}
