# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Stop a running vzdump task on node pve1. The task exits with a non-OK
# status afterwards.
action "pve_task_cancel" "stop_backup_task" {
  config {
    node = "pve1"
    upid = "UPID:pve1:0000ABCD:00000000:00000000:vzdump:100:root@pam:"
  }
}
