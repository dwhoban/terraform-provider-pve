resource "pve_vm_snapshot" "pre_upgrade" {
  node        = "pve1"
  vmid        = 100
  name        = "snap1"
  description = "Snapshot taken before upgrading the guest"
}

# Import an existing snapshot with:
# terraform import pve_vm_snapshot.pre_upgrade pve1/100/snap1
