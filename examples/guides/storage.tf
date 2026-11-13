# A directory storage on one node, serving ISO images and container
# templates.
resource "pve_storage" "iso" {
  storage = "iso-store"
  type    = "dir"
  path    = "/srv/iso"
  content = ["iso", "vztmpl"]
  nodes   = ["pve1"]
}

# Download an ISO onto it. The action runs when the storage exists.
resource "terraform_data" "iso_download" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_file_download_url.debian]
    }
  }
}

action "pve_file_download_url" "debian" {
  config {
    node     = "pve1"
    storage  = pve_storage.iso.storage
    filename = "debian-12.11.0-amd64-netinst.iso"
    url      = "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.11.0-amd64-netinst.iso"
    content  = "iso"
  }
}

# Inventory: what is already on a node's storages.
data "pve_node_storages" "pve1" {
  node = "pve1"
}
