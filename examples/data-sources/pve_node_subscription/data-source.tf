data "pve_node_subscription" "pve1" {
  node = "pve1"
}

output "subscription_status" {
  value = data.pve_node_subscription.pve1.status
}
