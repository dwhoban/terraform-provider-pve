// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase9 — Data protection & availability: backup jobs, replication, HA,
// realm-sync jobs, vzdump helpers.
var Phase9 = []Type{
	{
		Type: "pve_backup_job", Kind: "resource", Phase: 9,
		Summary: "Vzdump backup job definition (schedule, storage, guest selection).",
		Create:  &Endpoint{Path: "/cluster/backup"},
		Read:    &Endpoint{Path: "/cluster/backup/{id}"},
		Update:  &Endpoint{Path: "/cluster/backup/{id}"},
		Delete:  &Endpoint{Path: "/cluster/backup/{id}"},
		Attrs: []AttrRule{
			{Name: "item_id", ForceNew: true},
		},
	},
	{
		Type: "pve_backup_jobs", Kind: "data_source", Phase: 9,
		Summary: "Configured vzdump backup jobs; optionally the volumes included in one job.",
		Reads: []*Endpoint{
			{Path: "/cluster/backup", ResultName: "backup_jobs"},
			{Path: "/cluster/backup/{id}/included_volumes", ResultName: "included_volumes", SkipIfMissing: []string{"item_id"}},
		},
		Attrs: []AttrRule{
			{Name: "item_id", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_replication_job", Kind: "resource", Phase: 9,
		Summary: "Storage replication job between two nodes.",
		Create:  &Endpoint{Path: "/cluster/replication"},
		Read:    &Endpoint{Path: "/cluster/replication/{id}"},
		Update:  &Endpoint{Path: "/cluster/replication/{id}"},
		Delete:  &Endpoint{Path: "/cluster/replication/{id}"},
		Attrs: []AttrRule{
			{Name: "item_id", ForceNew: true},
		},
	},
	{
		Type: "pve_replication_jobs", Kind: "data_source", Phase: 9,
		Summary: "Replication jobs of a node; optionally one job's status and details.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/replication", ResultName: "jobs"},
			{Path: "/nodes/{node}/replication/{id}", ResultName: "job", SkipIfMissing: []string{"item_id"}},
			{Path: "/nodes/{node}/replication/{id}/status", ResultName: "status", SkipIfMissing: []string{"item_id"}},
		},
		Attrs: []AttrRule{
			{Name: "item_id", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_replication_schedule_now", Kind: "action", Phase: 9,
		Summary: "Schedule a replication job to run as soon as possible.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/replication/{id}/schedule_now"},
	},
	{
		Type: "pve_ha_group", Kind: "resource", Phase: 9,
		Summary: "HA group: a set of nodes with priorities for fencing guests.",
		Create:  &Endpoint{Path: "/cluster/ha/groups"},
		Read:    &Endpoint{Path: "/cluster/ha/groups/{group}"},
		Update:  &Endpoint{Path: "/cluster/ha/groups/{group}"},
		Delete:  &Endpoint{Path: "/cluster/ha/groups/{group}"},
		Attrs: []AttrRule{
			{Name: "group", PathParam: "group", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_ha_resource", Kind: "resource", Phase: 9,
		Summary: "HA-managed resource (guest) with restart/relocate constraints.",
		Create:  &Endpoint{Path: "/cluster/ha/resources"},
		Read:    &Endpoint{Path: "/cluster/ha/resources/{sid}"},
		Update:  &Endpoint{Path: "/cluster/ha/resources/{sid}"},
		Delete:  &Endpoint{Path: "/cluster/ha/resources/{sid}"},
		Attrs: []AttrRule{
			{Name: "sid", PathParam: "sid", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_ha_resource_migrate", Kind: "action", Phase: 9,
		Summary: "Migrate an HA resource to another node.",
		Invoke:  &Endpoint{Path: "/cluster/ha/resources/{sid}/migrate"},
	},
	{
		Type: "pve_ha_resource_relocate", Kind: "action", Phase: 9,
		Summary: "Relocate an HA resource to another node (with fencing semantics).",
		Invoke:  &Endpoint{Path: "/cluster/ha/resources/{sid}/relocate"},
	},
	{
		Type: "pve_ha_rule", Kind: "resource", Phase: 9,
		Summary: "HA constraint rule over resources.",
		Create:  &Endpoint{Path: "/cluster/ha/rules"},
		Read:    &Endpoint{Path: "/cluster/ha/rules/{rule}"},
		Update:  &Endpoint{Path: "/cluster/ha/rules/{rule}"},
		Delete:  &Endpoint{Path: "/cluster/ha/rules/{rule}"},
		Attrs: []AttrRule{
			{Name: "rule", Type: "string", Add: true, PathParam: "rule", Required: BoolPtr(true), ForceNew: true},
			{Name: "type", Required: BoolPtr(true), ForceNew: true},
			{Name: "resources", Type: "set", Elem: "string", Description: "HA resource IDs (e.g. `vm:100`, `ct:101`). PVE stores them sorted; a set keeps plans stable."},
			{Name: "nodes", Type: "set", Elem: "string", Description: "Nodes as `name` or `name:priority` (node-affinity only; required there). PVE drops `:0` priorities — write `pve03`, not `pve03:0`. Only valid when `type` is `node-affinity`."},
		},
	},
	{
		Type: "pve_ha_status", Kind: "data_source", Phase: 9,
		Summary: "HA cluster status: manager status, current resource states, and index.",
		Reads: []*Endpoint{
			{Path: "/cluster/ha", ResultName: "index"},
			{Path: "/cluster/ha/status", ResultName: "status"},
			{Path: "/cluster/ha/status/current", ResultName: "current"},
			{Path: "/cluster/ha/status/manager_status", ResultName: "manager_status"},
		},
	},
	{
		Type: "pve_ha_arm", Kind: "action", Phase: 9,
		Summary: "Arm the HA cluster (re-enable HA services after maintenance).",
		Invoke:  &Endpoint{Path: "/cluster/ha/status/arm-ha"},
	},
	{
		Type: "pve_ha_disarm", Kind: "action", Phase: 9,
		Summary: "Disarm the HA cluster for maintenance (stops HA services).",
		Invoke:  &Endpoint{Path: "/cluster/ha/status/disarm-ha"},
	},
	{
		Type: "pve_realm_sync_job", Kind: "resource", Phase: 9,
		Summary: "Realm-sync job syncing users/groups from an auth domain.",
		// apidoc: the POST create lives on the item family
		// /cluster/jobs/realm-sync/{id} (create-or-update semantics);
		// PUT on the same path updates.
		Create: &Endpoint{Path: "/cluster/jobs/realm-sync/{id}"},
		Read:   &Endpoint{Path: "/cluster/jobs/realm-sync/{id}"},
		Update: &Endpoint{Path: "/cluster/jobs/realm-sync/{id}"},
		Delete: &Endpoint{Path: "/cluster/jobs/realm-sync/{id}"},
	},
	{
		Type: "pve_cluster_jobs", Kind: "data_source", Phase: 9,
		Summary: "Cluster jobs index and configured realm-sync jobs.",
		Reads: []*Endpoint{
			{Path: "/cluster/jobs", ResultName: "jobs"},
			{Path: "/cluster/jobs/realm-sync", ResultName: "realm_sync_jobs"},
		},
	},
	{
		Type: "pve_vzdump_defaults", Kind: "data_source", Phase: 9,
		Summary: "Currently configured vzdump defaults of a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/vzdump/defaults", ResultName: "defaults"}},
	},
	{
		Type: "pve_vzdump_extractconfig", Kind: "data_source", Phase: 9,
		Summary: "Extract the guest configuration stored in a vzdump backup archive.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/vzdump/extractconfig", ResultName: "config"}},
	},
	{
		Type: "pve_backup_run", Kind: "action", Phase: 9,
		Summary: "Run a vzdump backup on a node (one-shot, waits for the task).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/vzdump"},
	},
	{
		Type: "pve_schedule_analyze", Kind: "function", Phase: 9,
		Summary: "Analyzes a job schedule and returns the next runtimes as a JSON-encoded array of {timestamp, utc} objects.",
		Call:    &Endpoint{Path: "/cluster/jobs/schedule-analyze"},
	},
}
