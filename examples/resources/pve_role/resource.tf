resource "pve_role" "operator" {
  roleid = "operator"
  privs = [
    "VM.Audit",
    "VM.Console",
    "VM.PowerMgmt",
    "VM.Migrate",
  ]
}
