resource "pve_notification_endpoint_webhook" "alerting" {
  name   = "alerting-hook"
  url    = "https://hooks.example.com/pve"
  method = "post"
  body   = "{\"text\":\"{{ title }}: {{ message }}\"}"
  headers = {
    "X-Token" = "static-header-value"
  }
  secrets = {
    HMAC = "your-signing-secret"
  }
  comment = "Webhook for the alerting pipeline"
}

