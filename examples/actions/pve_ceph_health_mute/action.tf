action "pve_ceph_health_mute" "example" {
  config {
  code = "CHANGE_ME"
  value = true
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_health_mute.example]
    }
  }
}
