# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# Download the Debian 12 standard appliance template onto the local
# storage of node pve1 and wait for the download to finish. Templates
# available on the node are listed by the pve_appliances data source.
action "pve_aplinfo_update" "fetch_debian12" {
  config {
    node     = "pve1"
    storage  = "local"
    template = "debian-12-standard_12.2-1_amd64.tar.zst"
  }
}
