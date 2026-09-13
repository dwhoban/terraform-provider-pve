data "pve_notification_matcher" "ops" {
  name = "ops"
}

output "matcher_targets" {
  value = data.pve_notification_matcher.ops.target
}
