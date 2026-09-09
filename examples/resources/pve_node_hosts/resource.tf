# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

resource "pve_node_hosts" "pve1" {
  node = "pve1"

  entries = [
    {
      address   = "127.0.0.1"
      hostnames = ["localhost.localdomain", "localhost"]
    },
    {
      address   = "192.168.1.10"
      hostnames = ["pve1.local", "pve1"]
    },
  ]
}
