# Copyright (c) HashiCorp, Inc.

data "pve_notification_endpoint_webhook" "alerting" {
  name = "alerting-hook"
}
