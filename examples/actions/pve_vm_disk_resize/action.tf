action "pve_vm_disk_resize" "example" {
  config {
  disk = "ide0"
  node = "pve1"
  size = "CHANGE_ME"
  vmid = 100
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_vm_disk_resize.example]
    }
  }
}
