action "pve_node_disk_wipe" "example" {
  config {
    disk = "CHANGE_ME"
    node = "pve1"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_node_disk_wipe.example]
    }
  }
}
