resource "pve_cluster_qdevice" "example" {
  node    = "pve1"
  address = "10.0.0.50"
  force   = true # needed for an odd node count
}
