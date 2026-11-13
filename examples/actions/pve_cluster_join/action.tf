action "pve_cluster_join" "example" {
  config {
  fingerprint = "CHANGE_ME"
  hostname = "CHANGE_ME"
  password = "CHANGE_ME"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_cluster_join.example]
    }
  }
}
