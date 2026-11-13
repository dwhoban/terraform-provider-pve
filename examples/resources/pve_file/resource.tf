resource "pve_file" "example" {
  filename = "CHANGE_ME"
  node     = "pve1"
  size     = "CHANGE_ME"
  storage  = "local"
  vmid     = 100
}
