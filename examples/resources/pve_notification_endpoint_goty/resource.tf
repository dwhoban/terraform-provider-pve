resource "pve_notification_endpoint_goty" "mobile_push" {
  name    = "gotify"
  server  = "https://gotify.example.com"
  token   = "your-gotify-application-token"
  comment = "Mobile push notifications"
}

