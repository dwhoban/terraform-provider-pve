# Copyright (c) HashiCorp, Inc.

# Recent tasks cluster wide.

data "pve_tasks" "recent" {}

output "failed_tasks" {
  value = [for t in data.pve_tasks.recent.tasks : t.upid if t.status != "OK" && t.status != ""]
}
