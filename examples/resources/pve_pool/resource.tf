resource "pve_pool" "prod" {
  poolid  = "prod"
  comment = "Production workload pool"

  vms      = [100]
  storages = ["local"]
}
