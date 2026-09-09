# Copyright (c) HashiCorp, Inc.

# Route error notifications from hosts matching `pve` to the built-in
# mail-to-root target; combine conditions with `any`.
resource "pve_notification_matcher" "ops" {
  name           = "ops"
  target         = ["mail-to-root"]
  match_field    = ["regex:hostname=^pve", "exact:severity=error"]
  match_severity = ["error", "warning"]
  mode           = "any"
  comment        = "Page the ops channel on errors"
}
