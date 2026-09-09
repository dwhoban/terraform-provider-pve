# Copyright (c) HashiCorp, Inc.

variable "influx_token" {
  type      = string
  sensitive = true
  default   = "replace-with-influxdb-api-token"
}

# InfluxDB metric server using the http v2 API.
resource "pve_metrics_server" "influx" {
  id                 = "influx1"
  type               = "influxdb"
  server             = "influx.example.com"
  port               = 8089
  influxdb_proto     = "http"
  organization       = "pve"
  bucket             = "proxmox"
  token              = var.influx_token
  verify_certificate = true
}

# Graphite metric server (UDP transport).
resource "pve_metrics_server" "graphite" {
  id      = "graphite1"
  type    = "graphite"
  server  = "graphite.example.com"
  port    = 2003
  proto   = "udp"
  path    = "proxmox.mycluster"
  timeout = 1
  mtu     = 1500
}
