# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    pve = {
      source = "hashicorp/pve"
    }
  }
}

# The next free VM ID in the cluster, as reported by the provider function.
# Call syntax: provider::<provider-name>::<function-name>().

output "next_vmid" {
  value = provider::pve::next_id()
}

# Example use: derive the ID of a new guest at plan time.
# resource "pve_vm" "example" {
#   node = "pve1"
#   name = "example"
#   # vmid = tonumber(provider::pve::next_id())
# }
