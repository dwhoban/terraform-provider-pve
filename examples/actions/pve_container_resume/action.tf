# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Resumes the suspended container 100 on pve1. Invoke with:
#   terraform apply -invoke pve_container_resume.resume_ct100
invoke "pve_container_resume" "resume_ct100" {
  config {
    node = "pve1"
    vmid = 100
  }
}
