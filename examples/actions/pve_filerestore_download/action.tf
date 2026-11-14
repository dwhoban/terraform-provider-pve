action "pve_filerestore_download" "example" {
  config {
  filepath = "CHANGE_ME"
  node = "pve1"
  storage = "local"
  volume = "CHANGE_ME"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_filerestore_download.example]
    }
  }
}
