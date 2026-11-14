resource "pve_container" "example" {
node = "pve1"
ostemplate = "CHANGE_ME"
vmid = 100
}
