action "pve_realm_sync" "example" {
  config {
    enable_new      = true
    full            = true
    purge           = true
    realm           = "CHANGE_ME"
    remove_vanished = "CHANGE_ME"
    scope           = "users"
  }
}

# terraform_data is a no-op stand-in; wire action.pve actions to the
# lifecycle events of any managed resource.
resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_realm_sync.example]
    }
  }
}
