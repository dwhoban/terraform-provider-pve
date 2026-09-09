# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Root-only arbitrary command execution on the node — validate the
# provenance of any configuration using this action. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_node_execute.run_batch]
action "pve_node_execute" "run_batch" {
  config {
    node = "pve1"
    commands = jsonencode([
      {
        method = "GET"
        path   = "version"
        args   = {}
      },
    ])
  }
}
