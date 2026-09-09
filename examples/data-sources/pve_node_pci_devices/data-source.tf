# Copyright (c) HashiCorp, Inc.

data "pve_node_pci_devices" "pve1" {
  node = "pve1"
}
