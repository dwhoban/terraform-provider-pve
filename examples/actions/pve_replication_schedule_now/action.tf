# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Start replication job 100-0 (guest 100) on node pve1 as soon as
# possible and wait for the replication task to finish.
action "pve_replication_schedule_now" "replicate_vm100_now" {
  config {
    node = "pve1"
    id   = "100-0"
  }
}
