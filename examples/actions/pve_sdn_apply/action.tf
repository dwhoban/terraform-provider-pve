# Copyright (c) HashiCorp, Inc.

# Apply every pending SDN change cluster-wide and wait for the reload task.
# Warning: this pushes all pending SDN configuration (zones, vnets,
# subnets, fabrics, controllers) to all nodes at once.
action "pve_sdn_apply" "apply_pending" {}
