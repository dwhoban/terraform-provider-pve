resource "pve_custom_cpu_model" "lab_cpu" {
  name           = "lab-cpu"
  reported_model = "Skylake-Client"
  flags          = "+pcid;+spec-ctrl"
  phys_bits      = "host"
}
