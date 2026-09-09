# Copyright (c) HashiCorp, Inc.

data "pve_notification_endpoint_sendmail" "ops_mail" {
  name = "ops-mail"
}
