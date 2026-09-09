# Copyright (c) HashiCorp, Inc.

resource "pve_hardware_mapping_pci" "gpu" {
  id          = "gpu"
  description = "Host GPU passed through to the media VM"
  mdev        = false

  map = [{
    node         = "pve1"
    id           = "10de:2231"
    iommugroup   = 14
    path         = "0000:01:00.0"
    subsystem_id = "1043:8888"
  }]
}
