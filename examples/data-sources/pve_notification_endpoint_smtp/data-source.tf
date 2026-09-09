# Copyright (c) HashiCorp, Inc.

data "pve_notification_endpoint_smtp" "relay" {
  name = "mailrelay"
}
