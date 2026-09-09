# Copyright (c) HashiCorp, Inc.

# Read the datacenter-wide cluster options. All attributes are computed;
# unset options are null.
data "pve_cluster_options" "options" {}

output "migration_settings" {
  value = data.pve_cluster_options.options.migration
}
