# Copyright (c) HashiCorp, Inc.

# Read the OpenFabric fabric including live per-node state from pve1.
data "pve_sdn_fabric_openfabric" "openfabric1" {
  fabric_id = "openfabric1"
  node      = "pve1"
}
