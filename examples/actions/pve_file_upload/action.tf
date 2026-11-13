action "pve_file_upload" "example" {
  config {
    content  = "iso"
    filename = "CHANGE_ME"
    node     = "pve1"
    storage  = "local"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_file_upload.example]
    }
  }
}
