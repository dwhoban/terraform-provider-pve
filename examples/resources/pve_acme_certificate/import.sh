# A node carries at most one ACME certificate; the import ID is the node
# name. Domains must also be present in configuration so the next plan
# matches.
terraform import pve_acme_certificate.le pve1
