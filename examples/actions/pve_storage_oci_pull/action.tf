# Copyright (c) HashiCorp, Inc.

# Pull the Alpine 3.20 OCI image from Docker Hub into the local storage of
# node pve1 and wait for the pull to finish.
action "pve_storage_oci_pull" "pull_alpine" {
  config {
    node      = "pve1"
    storage   = "local"
    reference = "docker.io/library/alpine:3.20"
  }
}
