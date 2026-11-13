# Requires the provider's ssh block: the Proxmox VE API cannot install packages.
action "pve_ceph_install" "pve1" {
  config {
    node       = "pve1"
    repository = "no-subscription"
  }
}

# Idempotent: nothing runs if Ceph is already installed on the node.
resource "terraform_data" "ceph_packages" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_ceph_install.pve1]
    }
  }
}
