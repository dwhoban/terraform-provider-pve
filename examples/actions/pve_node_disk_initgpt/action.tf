# Copyright (c) HashiCorp, Inc.

# Destructive: initializes /dev/sdb on node pve1 with a fresh GPT table,
# destroying any existing partition layout.
action "pve_node_disk_initgpt" "init_sdb" {
  config {
    node = "pve1"
    disk = "/dev/sdb"
  }
}
