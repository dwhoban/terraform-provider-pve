# Copyright (c) HashiCorp, Inc.

# Send a test notification through the built-in mail-to-root target to
# verify the notification chain end to end.
action "pve_notification_test" "smoke" {
  config {
    target = "mail-to-root"
  }
}
