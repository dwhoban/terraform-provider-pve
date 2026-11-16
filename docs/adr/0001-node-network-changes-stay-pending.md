# Node network changes stay pending until explicitly applied

PVE stages interface edits, and `PUT /nodes/{node}/network` reloads the whole node network. Applying after every resource reloaded the network once per interface. Mutations now only record pending changes. Commits are explicit: the `pve_node_network_apply` action (once per node), or the per-resource `apply_on_change` opt-in. Destroy never applies, because Terraform has no destroy-time action trigger and an implicit reload during teardown would be surprising. `pve_node_network_rollback` discards pending changes.
