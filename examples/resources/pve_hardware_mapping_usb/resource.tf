resource "pve_hardware_mapping_usb" "ups" {
  id          = "ups"
  description = "UPS serial link"

  map = [{
    node = "pve1"
    id   = "8087:0a2a"
    path = "1-2"
  }]
}
