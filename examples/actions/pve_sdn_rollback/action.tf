# Discard every pending SDN change cluster-wide, restoring the last applied
# configuration. Already applied configuration is not touched.
action "pve_sdn_rollback" "discard_pending" {}
