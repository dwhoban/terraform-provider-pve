# Disarm the HA stack during maintenance: resources are frozen (no new
# commands or state changes are applied) until the stack is armed again.
action "pve_ha_arm" "maintenance" {
  config {
    armed         = false
    resource_mode = "freeze"
  }
}

# Re-arm the HA stack afterwards:
# action "pve_ha_arm" "rearm" {
#   config {
#     armed = true
#   }
# }
