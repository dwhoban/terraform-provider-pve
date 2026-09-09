# Copyright (c) HashiCorp, Inc.

resource "pve_notification_endpoint_smtp" "relay" {
  name         = "mailrelay"
  server       = "smtp.example.com"
  from_address = "pve@example.com"
  username     = "mailer"
  password     = "your-smtp-password"
  port         = 587
  mode         = "starttls"
  mailto       = ["ops@example.com"]
  comment      = "SMTP relay for cluster notifications"
}

