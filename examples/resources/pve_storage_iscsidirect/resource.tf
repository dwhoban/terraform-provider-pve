# Manage an iSCSI direct-attached storage definition. Volumes are shared
# block devices; no snapshot or cloning is possible.
resource "pve_storage_iscsidirect" "fast" {
  storage = "fast"
  portal  = "192.168.1.10:3260"
  target  = "iqn.2000-01.com.example:fast.target0"

  nowritecache = true
  content      = ["images"]
}
