# Resume suspended VM 100 on node pve1.
action "pve_vm_resume" "resume_vm" {
  config {
    node = "pve1"
    vmid = 100
  }
}
