action "pve_ceph_restart_bulk" "example" {
  config {
    service_type = "mon"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_restart_bulk.example]
    }
  }
}
