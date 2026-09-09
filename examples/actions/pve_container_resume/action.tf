# Copyright (c) HashiCorp, Inc.

# Resumes the suspended container 100 on pve1. Invoke with:
#   apply it from a resource lifecycle block: actions = [action.pve_container_resume.resume_ct100]
action "pve_container_resume" "resume_ct100" {
  config {
    node = "pve1"
    vmid = 100
  }
}
