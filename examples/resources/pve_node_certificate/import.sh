# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

# A node carries at most one custom certificate; the import ID is the node
# name. Provide the chain and key in configuration so the next plan matches.
terraform import pve_node_certificate.custom pve1
