resource "pve_notification_endpoint_sendmail" "ops_mail" {
  name         = "ops-mail"
  mailto       = ["ops@example.com"]
  mailto_user  = ["root@pam"]
  from_address = "pve@example.com"
  author       = "PVE cluster"
  comment      = "Mail notifications for the ops team"
}

