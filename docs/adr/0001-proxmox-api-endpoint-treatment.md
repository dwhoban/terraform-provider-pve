# PVE API endpoint treatment map

We partition the complete Proxmox VE REST API surface (454 unique paths, pinned to the current `apidoc.js` from the PVE 9.x API viewer) into eight Terraform treatments using the rubric below, so that every endpoint has exactly one sanctioned answer and every future provider component has exactly one home. Type names are indicative `pve_`-prefixed snake_case following `internal/provider/resource_names.go`; the only naming changes decided here are the explicit rename/supersession rows (`cluster_nodes` → `pve_nodes`; `node_network_interface` → per-type family) — everything else awaits the dedicated naming decision.

Status: Accepted — 2026-09-09

## Rubric

| Code | Meaning | Terraform shape |
|---|---|---|
| R | Managed resource | `resource` with Create/Read/Update/Delete |
| D | Data source | `datasource`, read-only lookup (single or list) |
| A | Action | `action` block (TF ≥ 1.14), one-shot imperative op |
| E | Ephemeral resource | `ephemeral` block; short-lived secret/lease, never in state |
| F | Provider function | pure computation, no state |
| L | List resource | list-resource type managing an ordered collection as one object |
| I | Internal | consumed by `pveclient`/other components; no Terraform surface |
| X | Not exposed | deliberately outside the provider |

Combos written `R+D`. Default rubric:

- Named config entity with full CRUD → R (+D pair per policy below).
- Read-only aggregate/runtime snapshot → D.
- Imperative one-shot on an existing entity → A.
- Ordered mutable collection owned by a parent → L.
- Pure query/computation → F.
- Client plumbing (indexes, tunnels, task polling, capability probes) → I.
- Interactive consoles, log streams, RRD images, root-only exec, bootstrap/join flows → X.

Policies:

- **D-pair policy**: every R for a named entity implies an allowed same-name D (single + plural list) without a separate table row; explicit D rows appear only where D exists without R. This is why tables show few `R+D` rows.
- **Naming policy**: names are indicative, `pve_`-prefixed snake_case; existing registered names (`cluster_nodes`, `node_status`, `node_disks`, `node_network_interfaces`, `node`, `node_network_interface`, `node_disk_zfs`, `node_disk_lvm`) are respected as-is. Renaming is out of scope here.
- **Version pin**: inventory pinned to the current PVE 9.x API viewer `apidoc.js` (454 paths). Endpoints absent from the pin but present in upstream `.pm` sources are noted `verified-absent`; deprecated path forms are folded into their canonical row. Power ops follow the hybrid model: declarative `started` attribute on VM/CT resources, actions for one-shot ops, `node` attribute change performs migration.

## Decisions by domain

### Access

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /access/users`; `GET/PUT/DELETE /access/users/{userid}` | R `pve_user` | Named config entity, full CRUD (bpg precedent; confirmed B01) |
| `GET /access/users/{userid}/token[/{tokenid}]` (GET/POST/PUT/DELETE) | R `pve_user_token` | CRUD entity; secret is write-once Sensitive attr; E rejected — config outlives secret (confirmed B01) |
| `GET /access/tfa`; `GET/POST /access/tfa/{userid}`; `GET/PUT/DELETE /access/tfa/{userid}/{id}` | X | TFA secrets/recovery keys rotate out-of-band; round-trip badly through state (confirmed B02) |
| `PUT /access/users/{userid}/unlock-tfa` | A `pve_user_tfa_unlock` (P3) | One-shot unlock of a locked factor; niche (confirmed B02) |
| `PUT /access/password` | X | Self-service credential change, not desired-state (confirmed B02) |
| `GET /access/ticket` | X | Login-form helper returning dummy ticket (confirmed B02) |
| `POST /access/ticket` | I | `pveclient` password→ticket auth path (auth.go) (confirmed B02) |
| `POST /access/vncticket` | X | Interactive console auth only (confirmed B02) |
| `GET /access/permissions` | D `pve_permissions` | Effective-permission lookup for precondition asserts (confirmed B02) |
| `GET /access` | I | Directory listing, client plumbing (confirmed B02) |
| `GET/POST /access/groups`; `GET/PUT/DELETE /access/groups/{groupid}` | R `pve_group` | Full CRUD entity (confirmed B03) |
| `GET/POST /access/roles`; `GET/PUT/DELETE /access/roles/{roleid}` | R `pve_role` | Full CRUD entity (confirmed B03) |
| `GET/PUT /access/acl` | R `pve_acl` (per entry) | PUT upserts one entry per call; per-entry resource matches bpg; L whole-set considered and rejected (confirmed B03) |
| `GET/POST /access/domains`; `GET/PUT/DELETE /access/domains/{realm}` | R `pve_realm_ldap` / `pve_realm_ad` / `pve_realm_openid` (+ D `pve_realms` incl. built-ins) | Per-type resources keep divergent schemas honest; single type-discriminated R considered and rejected (confirmed B04) |
| `POST /access/domains/{realm}/sync` | A `pve_realm_sync` | Ad-hoc imperative sync with dry-run param (confirmed B04) |
| `GET/POST /cluster/jobs/realm-sync[/{id}]` | R `pve_realm_sync_job` | Scheduled job is a config entity (confirmed B04) |
| `GET /cluster/jobs/schedule-analyze` | I | Schedule-parsing helper for schema validation (confirmed B04) |
| `GET /access/openid`; `POST /access/openid/{auth-url,login}` | X | Interactive browser login flow (confirmed B04) |

### Cluster core

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /cluster` | I | Directory listing (confirmed B05) |
| `GET /cluster/resources` | D `pve_cluster_resources` | Cluster-wide lookup table, type-filterable (confirmed B05) |
| `GET /cluster/status` | D `pve_cluster_status` | Quorum/topology; absorbs `config/totem` + `config/qdevice` reads as attrs (confirmed B05) |
| `GET /cluster/config/totem`; `GET /cluster/config/qdevice` | D attrs of `pve_cluster_status` | Read-only facts (confirmed B05) |
| `GET /cluster/tasks` | D `pve_tasks` | Cluster-wide task list (confirmed B05) |
| `GET /cluster/nextid` | F `next_id()` | Pure VMID allocation query; D alternative considered and rejected (confirmed B05) |
| `GET /cluster/backup-info/not-backed-up` | D attr of `pve_backup_jobs` | Backup coverage audit read (confirmed B05) |
| `POST /cluster/config` (create cluster) | X | Bootstrap on a standalone node is interactive/irreversible; out of provider scope (confirmed B06) |
| `POST /cluster/config/join`; `POST/DELETE /cluster/config/nodes/{node}` | R `pve_cluster_node` | USER OVERRIDE: node membership must be manageable — create = join/add node, delete = remove node; join info (GET) feeds the implementation (override B06) |
| `GET /cluster/config/join` | I | Join info consumed by `pve_cluster_node` create (confirmed B06 override) |
| `GET/PUT /cluster/options` | R `pve_cluster_options` | Singleton cluster-wide config entity (confirmed B06) |

### HA
| Endpoints | Treatment | Rationale |
| `GET /cluster/ha`; `GET /cluster/ha/status`; `GET /cluster/ha/status/{current,manager_status}` | D `pve_ha_status` | HA runtime snapshot incl. LRM/CRM state (confirmed B07) |
| `POST /cluster/ha/status/{arm-ha,disarm-ha}` | A `pve_ha_arm` | USER OVERRIDE: cluster-wide HA maintenance toggle exposed as one-shot action (override B07) |
| `GET/POST /cluster/ha/groups`; `GET/PUT/DELETE /cluster/ha/groups/{group}` | R `pve_ha_group` | HA node-group config entity (confirmed B07) |
| `GET/POST /cluster/ha/resources`; `GET/PUT/DELETE /cluster/ha/resources/{sid}` | R `pve_ha_resource` | Binds guest to HA group with failover policy (confirmed B07) |
| `GET/POST /cluster/ha/rules`; `GET/PUT/DELETE /cluster/ha/rules/{rule}` | R `pve_ha_rule` | HA constraint rules entity (confirmed B07) |
| `POST /cluster/ha/resources/{sid}/{migrate,relocate}` | A `pve_ha_resource_migrate` / `pve_ha_resource_relocate` (P3) | One-shot imperative HA placement ops (confirmed B07) |
| `/nodes/{node}/ha` | verified-absent | Not in pinned apidoc.js; upstream pve-ha-manager registers it; ops map to resource-level migrate/relocate (confirmed B07) |

### Firewall
| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/PUT /cluster/firewall/options` | R `pve_cluster_firewall_options` | Singleton cluster-scope firewall config (confirmed B08) |
| `GET/POST /cluster/firewall/aliases`; `GET/PUT/DELETE .../aliases/{name}` | R `pve_firewall_alias` | Named network-alias entity (confirmed B08) |
| `GET/POST /cluster/firewall/ipset`; `.../ipset/{name}[/{cidr}]` | R `pve_firewall_ipset` w/ inline `cidrs` set | Entries are an unordered set; inline avoids reorder churn; separate L rejected (confirmed B08) |
| `GET/POST /cluster/firewall/groups`; `GET/POST/DELETE .../groups/{group}` | R `pve_firewall_security_group` | Named reusable rule-collection entity (confirmed B08) |
| `GET /cluster/firewall/{macros,refs}` | I | Validation enums for schema validators (confirmed B08) |
| `GET /nodes/{node}/firewall/log` | X | Log stream; cluster-scope log absent from pin (path-corrected, B08) |
| `.../{cluster firewall, nodes/{node}/firewall, qemu|lxc/{vmid}/firewall, groups/{group}, sdn/vnets/{vnet}/firewall}/rules[/{pos}]` (GET/POST/PUT/DELETE) | L `pve_{cluster,node,guest}_firewall_rules` + group-keyed and vnet-keyed variants | Ordered `pos`-indexed collection = textbook list resource; per-rule R reindex churn and inline-block drift merge both rejected (confirmed B09) |
| `GET/PUT /nodes/{node}/firewall/options`; `GET/PUT /nodes/{node}/{qemu,lxc}/{vmid}/firewall/options` | R `pve_node_firewall_options` / `pve_guest_firewall_options` | Per-scope singletons mirroring cluster options R (confirmed B09) |
| `GET/PUT /cluster/sdn/vnets/{vnet}/firewall/options` | R `pve_sdn_firewall_options` (vnet-keyed) | Mirrors B09 ruling at vnet scope (confirmed B09) |
| `/nodes/{node}/{qemu,lxc}/{vmid}/firewall/{aliases[/{name}],ipset[/{name}[/{cidr}]]}` | X | Guest-scope defs rarely used; cluster-scope covers shared sets (confirmed B09) |
| `GET /nodes/{node}/{qemu,lxc}/{vmid}/firewall/refs` | I | Validation enums (confirmed B09) |
| `GET /nodes/{node}/{qemu,lxc}/{vmid}/firewall/log` | X | Log stream (confirmed B09) |

### Backup & replication
| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /cluster/backup`; `GET/PUT/DELETE /cluster/backup/{id}` | R `pve_backup_job` | Scheduled vzdump job config entity (confirmed B10) |
| `GET /cluster/backup/{id}/included_volumes` | D attr | Guest-coverage read (confirmed B10) |
| `GET/POST /cluster/replication`; `GET/PUT/DELETE /cluster/replication/{id}` | R `pve_replication` | pvesdr job config entity (confirmed B10) |
| `GET /nodes/{node}/replication`; `GET .../{id}`; `GET .../{id}/status` | D `pve_node_replications` (+ status attrs) | Runtime replication state (confirmed B10) |
| `GET /nodes/{node}/replication/{id}/log` | D attr | Replication log read (confirmed B10) |
| `POST /nodes/{node}/replication/{id}/schedule_now` | A `pve_replication_schedule_now` (P3) | One-shot immediate replication run (confirmed B10) |
| `GET /nodes/{node}/vzdump/defaults` | D attr | Node vzdump defaults feed job authoring (confirmed B10) |
| `POST /nodes/{node}/vzdump` | A `pve_backup_run` (P3) | Ad-hoc imperative backup; X alternative rejected — escape hatch has value (confirmed B10) |
| `POST /nodes/{node}/vzdump/extractconfig` | I | Backup-config extraction helper (confirmed B10) |

### Metrics & notifications

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /cluster/metrics/server`; `GET/PUT/DELETE .../server/{id}` | R `pve_metrics_server` | Metrics-server config entity (confirmed B11) |
| `GET /cluster/metrics/export` | X | Telemetry scrape, not config (confirmed B11) |
| `GET /cluster/metrics`; `GET /cluster/notifications` | I | Directory listings (confirmed B11) |
| `/cluster/notifications/endpoints/{sendmail,gotify,smtp,webhook}` (+ `/{name}` CRUD) | R `pve_notification_endpoint_{sendmail,gotify,smtp,webhook}` | Divergent field sets per type; single typed R rejected (confirmed B11) |
| `/cluster/notifications/matchers` (+ `/{name}` CRUD) | R `pve_notification_matcher` | Routing rules entity (confirmed B11) |
| `GET /cluster/notifications/targets` | D `pve_notification_targets` | Aggregated targets incl. built-ins (confirmed B11) |
| `POST /cluster/notifications/targets/{name}/test` | A `pve_notification_test` | One-shot test send (confirmed B11) |
| `GET /cluster/notifications/{matcher-fields,matcher-field-values}` | I | Enums for matcher schema validators (accepted by default — question cancelled B11) |
### SDN

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /cluster/sdn/zones`; `GET/PUT/DELETE .../zones/{zone}` | R `pve_sdn_zone_{simple,vlan,qinq,vxlan,evpn}` | Divergent fields per type; single typed R rejected (confirmed B12) |
| `GET/POST /cluster/sdn/vnets`; `GET/PUT/DELETE .../vnets/{vnet}` | R `pve_sdn_vnet` | VNet config entity (confirmed B12) |
| `GET/POST /cluster/sdn/vnets/{vnet}/subnets`; `GET/PUT/DELETE .../subnets/{subnet}` | R `pve_sdn_subnet` | Subnet entity keyed by vnet (confirmed B12) |
| `GET/POST /cluster/sdn/controllers`; `GET/PUT/DELETE .../controllers/{controller}` | R `pve_sdn_controller` | (evpn) controller entity (confirmed B12) |
| `GET/POST /cluster/sdn/dns`; `GET/PUT/DELETE .../dns/{dns}` | R `pve_sdn_dns` | SDN DNS-config entity (diff find; confirmed B12) |
| `GET/POST /cluster/sdn/ipams`; `.../ipams/{ipam}` CRUD; `GET .../ipams/{ipam}/status` | R `pve_sdn_ipam` (+ D status attrs) | IPAM config entity + runtime status (diff find; confirmed B12) |
| `/cluster/sdn/prefix-lists` + `/{id}` CRUD + `entries` + `entries/{url_seq}` | R `pve_sdn_prefix_list` w/ ordered inline entries | Entries are ordered; separate L rejected as over-modeling (diff find; confirmed B12) |
| `/cluster/sdn/route-maps` + `entries/{route-map-id}/entry/{order}` family | R `pve_sdn_route_map` w/ ordered inline entries | Mirrors prefix-lists (diff find; confirmed B12) |
| `/cluster/sdn/fabrics/{fabric,node}` families | R `pve_sdn_fabric_{ospf,openfabric}` (+ node members) (P3) | Real entities; new and deep, built last (diff find; confirmed B12) |
| `PUT /cluster/sdn` (apply) | A `pve_sdn_apply` | Two-phase commit is imperative; applier-R (bpg) rejected — trigger churn in state (confirmed B12) |
| `POST /cluster/sdn/rollback` | A `pve_sdn_rollback` (P3) | One-shot rollback (diff find; confirmed B12) |
| `GET /cluster/sdn/dry-run`; `POST/DELETE /cluster/sdn/lock` | I | Apply preview + internal lock (diff find; confirmed B12) |
| `GET /cluster/sdn/vnets/{vnet}/ips` | D attrs | Per-vnet IP usage facts (diff find; confirmed B12) |
| `GET /nodes/{node}/sdn*` runtime GETs (zones bridges/content/ip-vrf; vnets mac-vrf; fabrics interfaces/neighbors/routes) | D attrs | Node-scope runtime state (diff find; confirmed B12) |
| `GET /cluster/sdn`; `GET /cluster/sdn/{zones,vnets,...}` listings | I | Directory listings (confirmed B12) |

### ACME, certificates & mappings

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /cluster/acme/account`; `GET/PUT/DELETE .../account/{name}` | R `pve_acme_account` | ACME account entity (confirmed B13) |
| `GET /cluster/acme/plugins`; `GET/PUT/DELETE .../plugins/{id}` | D `pve_acme_plugins` + R `pve_acme_dns_plugin` | Installed list read-only; custom DNS plugin is config (confirmed B13) |
| `GET /cluster/acme/{challenge-schema,directories,meta,tos}` | I | Enums/UX helpers for ACME schema (diff-corrected from 'challenges'; confirmed B13) |
| `/cluster/mapping/{pci,usb,dir}` (+ `/{id}` CRUD) | R `pve_hardware_mapping_{pci,usb}` + `pve_mapping_dir` | Mapping entities; dir type found in diff (confirmed B13) |
| `GET /cluster/qemu/cpu-flags` | I | CPU-flag enum for VM schema validators (confirmed B13) |
| `/cluster/qemu/custom-cpu-models` (+ `/{cputype}` CRUD) | R `pve_custom_cpu_model` | Custom CPU model entity (confirmed B13) |
| `GET /nodes/{node}/certificates/info` | D attrs | Node TLS facts (confirmed B13) |
| `POST/DELETE /nodes/{node}/certificates/custom` | R `pve_node_certificate` | Custom certificate upload path (confirmed B13) |
| `POST /nodes/{node}/certificates/acme/certificate` (+ acme node opts) | R `pve_acme_certificate` | ACME order/renew lifecycle (confirmed B13) |

### Node core

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes` | D `pve_nodes` | USER OVERRIDE: make it symmetrical — existing `cluster_nodes` data source renames to `pve_nodes`; broader naming audit of existing components noted as follow-up (override B14) |
| `GET /nodes/{node}` (index) | I | Per-node directory listing (confirmed B14) |
| `GET /nodes/{node}/status` | D `pve_node_status` | Runtime snapshot; exists (confirmed B14) |
| `POST /nodes/{node}/status` (reboot/shutdown) | A `pve_node_reboot` / `pve_node_shutdown` | One-shot with blast radius; explicit opt-in (confirmed B14) |
| `GET /nodes/{node}/version` | I | pveversion capability probe (confirmed B14) |
| `GET/PUT /nodes/{node}/config` | R `pve_node_config` | node.cfg keys entity; client exists (confirmed B14) |
| `GET/PUT /nodes/{node}/dns`; `GET/POST /nodes/{node}/hosts`; `GET/PUT /nodes/{node}/time` | R `pve_node_{dns,hosts,time}` | Singleton config entities; dns/time clients exist (confirmed B14) |
| `GET /nodes/{node}/subscription` | D `pve_node_subscription` | Licensing facts (confirmed B14) |
| `POST/PUT/DELETE /nodes/{node}/subscription` (methods live on the path itself; no children in pin) | A `pve_subscription_refresh` (P3); PUT/DELETE (set/remove key) → X | POST = refresh one-shot; key management out-of-band (path-corrected from apidoc pin; confirmed B14) |
### Node telemetry & ops

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/{syslog,journal,rrd,rrddata,netstat,report}`; `GET /cluster/log` | X | Operational streams/images, not desired-state (confirmed B15) |
| `POST /nodes/{node}/wakeonlan` | A `pve_node_wakeonlan` | One-shot wake packet (confirmed B15) |
| `POST /nodes/{node}/{startall,stopall,suspendall,migrateall}` | A family (P3) e.g. `pve_node_start_all` | Bulk escape hatches, explicit opt-in (confirmed B15) |
| `POST /cluster/bulk-action/guest/{start,shutdown,suspend,migrate}` | A family (P3) e.g. `pve_guest_bulk_start` | Cluster-wide guest bulk ops, mirrors node ruling (diff find; confirmed B15) |
| `POST /nodes/{node}/execute` | X  |  |
| `/nodes/{node}/{vncshell,termproxy,vncwebsocket,spiceshell}` | X | Interactive consoles (confirmed B15) |
### Node network

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/network` | D `node_network_interfaces` | Exists in repo; naming audit noted (confirmed B16) |
| `POST /nodes/{node}/network`; `GET/PUT/DELETE /nodes/{node}/network/{iface}` | R per type: `node_network_linux_bridge` / `node_network_linux_bond` / `node_network_vlan` | USER DECISION: fields differ per type even though the API is one endpoint family; mirrors bpg's linux_{bridge,bond,vlan} split. Existing single `node_network_interface` resource is superseded by this family (override B16) |
| Network apply (POST on collection root) | folded into R | Apply is the commit phase of the same change (confirmed B16) |
| Network revert (DELETE on collection root) | I | Discard-pending lives inside the resource (confirmed B16) |

### Node services, apt & tasks

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/services` | D `pve_node_services` | Service state facts (confirmed B17) |
| `POST /nodes/{node}/services/{service}/{start,stop,restart,reload}`; `GET .../state` | A `pve_node_service` w/ `operation` enum | One action covers four verbs; state reads via the D (confirmed B17) |
| `GET /nodes/{node}/apt/{repositories,versions,changelog}` | D `pve_node_apt_repositories` + version attrs (changelog P3) | Package-manager facts (confirmed B17) |
| `POST /nodes/{node}/apt/update` | A `pve_node_apt_update` | One-shot package-index refresh (confirmed B17) |
| `GET/POST /nodes/{node}/apt/repositories` (add standard repo) | R `pve_apt_standard_repository` | Enabling a standard repo is desired-state (confirmed B17) |
| `GET /nodes/{node}/tasks` | D `pve_node_tasks` | Node task list (confirmed B17) |
| `GET /nodes/{node}/tasks/{upid}/{status,log}` | I | WaitForTask polling plumbing (confirmed B17) |
| `DELETE /nodes/{node}/tasks/{upid}` (cancel) | A `pve_task_cancel` (P3) | One-shot cancel; apidoc pin has no /stop child — DELETE is the verb (diff-corrected; confirmed B17) |
| `GET /nodes/{node}/scan/{nfs,cifs,iscsi,lvm,lvmthin,zfs,pbs}` | I | Storage-creation discovery helpers (confirmed B17) |
| `GET /nodes/{node}/aplinfo` (+ POST update) | D `pve_appliances` (P3) + A `pve_aplinfo_update` (P3) | CT-template catalog + refresh (diff find; confirmed B17) |
| `GET /nodes/{node}/{query-url-metadata,query-oci-repo-tags}` | I | Download/OCI metadata probing (diff find; confirmed B17) |
| `GET /nodes/{node}/hardware/{pci,usb}` (+ `pci/{id}/mdev`) | D `pve_node_pci_devices` / `pve_node_usb_devices` (+ mdev attrs) | Passthrough planning facts (confirmed B17) |
| `GET /nodes/{node}/capabilities/qemu/{cpu,cpu-flags,machines,migration}` | D `pve_node_capabilities` | VM authoring facts (confirmed B17) |

### Node disks & ceph

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/disks/list`; `GET .../disks/{zfs,lvm,lvmthin,directory}`; `GET .../disks/smart` | D `node_disks` (+ smart/listing attrs) | Disk inventory facts; exists (confirmed B18) |
| `POST /nodes/{node}/disks/initgpt` | A (P3) | One-shot GPT init before disk use (confirmed B18) |
| `POST /nodes/{node}/disks/wipedisk` | X | Destructive manual op; destruction belongs to resource Delete (confirmed B18) |
| `POST /nodes/{node}/disks/{zfs,lvm,lvmthin,directory}`; `GET/DELETE .../{fs}/{name}` | R `node_disk_zfs` (exists) / `node_disk_lvm` (exists) / `node_disk_lvmthin` / `node_disk_directory` | Filesystem-on-disk entities, existing family (confirmed B18) |
| `/nodes/{node}/ceph` status GETs | D `pve_ceph_status` | Runtime facts (override B18) |
| `/nodes/{node}/ceph/pool*` CRUD | R `pve_ceph_pool` | Pool entity (override B18) |
| `/nodes/{node}/ceph/{osd,mon}` CRUD/verbs | R `pve_ceph_osd` / `pve_ceph_mon` | USER OVERRIDE: broader R coverage — osd/mon as resource families (override B18) |
| `/nodes/{node}/ceph/{mgr,mds,fs,flags,...}` | X | Transitional admin surface beyond the override (override B18) |
| `/cluster/ceph/{status,metadata}` | D attrs of `pve_ceph_status` | Mirrors node ruling (diff find; confirmed B18) |
| `/cluster/ceph/{flags,flags/{flag},health-mute,health-mute/{code},restart-bulk}` | X | Admin toggles (diff find; confirmed B18) |

### QEMU core

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/qemu` | D `pve_vms` | Guest list lookup (confirmed B19) |
| `POST /nodes/{node}/qemu`; `DELETE .../{vmid}`; `GET/POST/PUT .../{vmid}/config`; `GET .../{vmid}/pending` | R `pve_vm` (+ D single) | Flagship resource; pending drives drift detection (confirmed B19) |
| `GET .../{vmid}/status/current` | D attrs | Runtime state (confirmed B19) |
| `POST .../{vmid}/status/{start,stop,shutdown}` | `started` attribute (+ `stop_on_destroy`, timeout/force) | Hybrid power model (confirmed B19) |
| `POST .../{vmid}/status/{reboot,suspend,resume,reset}` | A `pve_vm_{reboot,suspend,resume,reset}` | One-shot power ops (confirmed B19) |
| `POST .../{vmid}/clone` | `clone` create-block on `pve_vm` | Clone is a creation source; separate cloned_vm R (bpg) rejected (confirmed B19) |
| `POST .../{vmid}/template` | `template` bool attribute (ForceNew) | One-way conversion is config (confirmed B19) |
| `GET .../{vmid}/migrate` | I | Precondition probe (confirmed B19) |
| `POST .../{vmid}/migrate` | `node` attr change (R Update) + A `pve_vm_migrate` escape hatch | Hybrid model; dual path noted — action is for on-demand migrate without config change (confirmed B19) |
| `POST .../{vmid}/remote_migrate` | X | Experimental cross-cluster migration (confirmed B19) |
| `PUT .../{vmid}/resize`; `POST .../{vmid}/move_disk` | folded into R disk blocks | size/storage changes drive resize/move in Update (confirmed B19) |
| `PUT .../{vmid}/unlink`; `GET .../{vmid}/feature` | I | Config plumbing / precondition check (confirmed B19) |
| `GET/PUT .../{vmid}/cloudinit`; `GET .../{vmid}/cloudinit/dump` | `cloud_init` block on `pve_vm`; dump → I | ipconfigN are config props (confirmed B19) |

### QEMU snapshots, agent & misc

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST .../{vmid}/snapshot`; `GET/PUT/DELETE .../snapshot/{snapname}[/config]` | R `pve_vm_snapshot` | Snapshot entity with description metadata (confirmed B20) |
| `POST .../{vmid}/snapshot/{snapname}/rollback` | A `pve_vm_snapshot_rollback` | Destructive one-shot, explicit opt-in (confirmed B20) |
| `GET .../{vmid}/agent` (index) | I | Directory listing (confirmed B20) |
| `POST .../{vmid}/agent` (generic command) | X | Superseded by named endpoints (confirmed B20) |
| 11 read-only agent GETs (`info`, `get-{time,osinfo,host-name,users,timezone,vcpus,fsinfo,memory-blocks,memory-block-info}`, `network-get-interfaces`) | D `pve_vm_agent_info` (P3) | Guest introspection; network-get-interfaces is the IP-discovery path (confirmed B20) |
| `POST .../{vmid}/agent/ping` | I | Connectivity wait helper (confirmed B20) |
| Mutating agent ops (`fsfreeze-*`, `fstrim`, `suspend-*`, `shutdown`, `set-user-password`, `exec`, `file-write`) + `GET {exec-status,file-read}` | X | Guest-internal mutation belongs to provisioning tools (confirmed B20) |
| `.../{vmid}/{vncproxy,termproxy,vncwebsocket,spiceproxy,sendkey,monitor}` | X | Interactive console surface (confirmed B20) |
| `GET .../{vmid}/{rrd,rrddata}` | X | Guest telemetry (confirmed B20) |
| `.../{vmid}/{mtunnel,mtunnelwebsocket,dbus-vmstate}` | I | Tunnel plumbing (confirmed B20) |

### LXC

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET /nodes/{node}/lxc` | D `pve_containers` | Guest list lookup (confirmed B21) |
| `POST /nodes/{node}/lxc`; `DELETE .../{vmid}`; `GET/POST/PUT .../{vmid}/config`; `GET .../{vmid}/pending` | R `pve_container` (+ D single) | Flagship container resource, mirrors `pve_vm` (confirmed B21) |
| `GET .../{vmid}/status/current` | D attrs | Runtime state (confirmed B21) |
| `POST .../{vmid}/status/{start,stop,shutdown}` | `started` attribute | Hybrid power model, mirrors B19 (confirmed B21) |
| `POST .../{vmid}/status/{reboot,suspend,resume}` | A `pve_container_{reboot,suspend,resume}` | One-shot ops; LXC has no reset (confirmed B21) |
| `POST .../{vmid}/clone` | `clone` create-block | Mirrors B19 (confirmed B21) |
| `POST .../{vmid}/template` | `template` bool attribute (ForceNew) | Mirrors B19 (confirmed B21) |
| `GET .../{vmid}/migrate` | I | Precondition probe (confirmed B21) |
| `POST .../{vmid}/migrate` | `node` attr change + A `pve_container_migrate` escape hatch | Mirrors B19 (confirmed B21) |
| `POST .../{vmid}/remote_migrate` | X | Experimental (confirmed B21) |
| `PUT .../{vmid}/resize`; `POST .../{vmid}/move_volume` | folded into R mount-point blocks | mpN size/storage changes drive resize/move_volume (confirmed B21) |
| `GET .../{vmid}/interfaces` | D attrs | Container IP discovery, no agent dependency (confirmed B21) |
| `.../{vmid}/snapshot` family (+ `{snapname}/rollback`) | R `pve_container_snapshot` + A `pve_container_snapshot_rollback` | Mirrors B20 (confirmed B21) |
| `GET .../{vmid}/feature` | I | Precondition check (confirmed B21) |
| `.../{vmid}/{vncproxy,termproxy,vncwebsocket,spiceproxy}`; `GET .../{vmid}/{rrd,rrddata}` | X | Consoles + telemetry (confirmed B21) |
| `.../{vmid}/{mtunnel,mtunnelwebsocket}` | I | Tunnel plumbing (confirmed B21) |

### Storage, pools & version

| Endpoints | Treatment | Rationale |
|---|---|---|
| `GET/POST /storage`; `GET/PUT/DELETE /storage/{storage}` | R per type: `pve_storage_{nfs,cifs,iscsi,iscsidirect,lvm,lvmthin,zfspool,directory,pbs,cephfs,rbd,glusterfs}` | Type-divergent schemas; matches node_disk_* family; single typed R rejected (confirmed B22) |
| `GET /nodes/{node}/storage`; `GET .../storage/{storage}` (status) | D `pve_node_storages` (+ status attrs) | Per-node storage facts (confirmed B22) |
| `GET .../storage/{storage}/content` | D `pve_storage_files` | Volume listing (confirmed B22) |
| `POST .../content` (upload); `GET/PUT/DELETE .../content/{volume}` | R `pve_file` | Uploaded-file entity incl. ISO/snippet/template (confirmed B22) |
| `POST .../storage/{storage}/download-url` | R `pve_download_file` | Distinct URL-pull create path with checksums (confirmed B22) |
| `POST .../storage/{storage}/oci-registry-pull` | A (P3) | One-shot OCI pull; R if usage grows (diff find; confirmed B22) |
| `GET/PUT .../storage/{storage}/prunebackups` | A `pve_storage_prune_backups` (dry-run param) | Imperative retention op (confirmed B22) |
| `.../storage/{storage}/file-restore/{list,download}` | X | Interactive restore UX (confirmed B22) |
| `GET .../storage/{storage}/{identity,status}`; `GET .../storage/{storage}/import-metadata`; `GET .../storage/{storage}/{rrd,rrddata}` | D attrs; I; X | Diff finds: PBS identity + status facts, import plumbing, telemetry (confirmed B22) |
| `GET/POST /pools`; `PUT/DELETE /pools/{poolid}` (deprecated verb forms folded) | R `pve_pool` w/ `members` list attr | Simplest atomic model; separate membership R (bpg) rejected (confirmed B22) |
| `GET /version` | D `pve_version` | Version/parity read (confirmed B22) |

## Considered options

- **Taxonomy size**: 7 vs 8 values — L (list resource) added for ordered mutable collections; first exercised by firewall rules (B09).
- **Guest power**: attribute-only vs action-only vs hybrid — hybrid chosen: declarative `started`/`node` attributes carry lifecycle; actions cover reboot/suspend/resume/reset and on-demand migrate (B19, mirrored B21).
- **Firewall rules**: per-rule R vs inline block vs L — L per scope; reindex churn and drift-merge complexity rejected (B09).
- **ACL**: per-entry R vs whole-set L — per-entry R; API upserts one entry per PUT (B03).
- **Clone**: create-block on the guest R vs separate cloned-vm R — create-block; clone is a creation source, not an entity (B19).
- **Per-type vs single typed R**: applied uniformly per-type for realms, notification endpoints, SDN zones, storages, and network interfaces — divergent field sets beat one conditional schema (B04, B11, B12, B16, B22).
- **Cluster formation**: full X vs exposure — user overrode to expose membership as `pve_cluster_node`; create-cluster stays X (B06).
- **TFA management**: R vs X — X; rotation semantics round-trip badly through state (B02).
- **`/cluster/nextid`**: F vs D — F `next_id()`; pure allocation query (B05).
- **SDN apply**: applier-R (bpg) vs A — A; two-phase commit is imperative (B12).
- **Ceph scope**: minimal (status D + pool R) vs broader — user overrode to broader; osd/mon also become R families (B18).
- **`/nodes/{node}/execute`**: X (security) vs A — user overrode to A despite the root-only arbitrary-exec risk; see Consequences (B15).
- **bpg compatibility**: parity vs greenfield — greenfield with precedent: we deviate deliberately (clone-block, SDN apply action, no dual/legacy names).

## Consequences

- Actions require Terraform ≥ 1.14; registration must keep the existing version gates (repo AGENTS.md).
- E (ephemeral) is unused in v1; reserved for lease-like secrets if any appear.
- L is the least-proven shape here; it is deliberately confined to ordered collections (firewall rules at every scope). Revisit after the first L ships.
- `pve_node_execute` exposes root-only arbitrary execution as an action — accepted by explicit decision; its implementation must carry prominent schema warnings and never echo secrets.
- Two component changes follow directly: rename `cluster_nodes` → `pve_nodes`; supersede `node_network_interface` with the per-type family. A broader naming audit of existing components is a recorded follow-up (B14 note).
- P3 markers are priority-3 (build-late) signals, not scope cuts; every P3 row is still a sanctioned component.
- Drift policy: endpoints added in future PVE releases get a treatment via the rubric plus a revision of this ADR — never silent omission. Conditional subtrees (HA, SDN, Ceph, ACME, jobs) verified against upstream `.pm` sources are in scope on clusters where the packages are installed.

## References

- Inventory pin: `https://pve.proxmox.com/pve-docs/api-viewer/apidoc.js` (current PVE 9.x viewer; 454 unique paths; fetched 2026-09-09).
- Upstream Perl sources for conditionally-mounted subtrees and semantics: `pve-manager` (Cluster.pm, Nodes.pm), `pve-ha-manager`, `pve-network` (SDN), `pve-storage` (Config/Status/Content/PruneBackups/FileRestore), `qemu-server` (Qemu.pm, Agent.pm), `pve-container` (LXC*.pm), `pve-firewall`, `pve-access-control`.
- Peer precedents surveyed: Telmate/terraform-provider-proxmox (provider.go:204–221 registration map) and bpg/terraform-provider-proxmox (`fwprovider/` + `proxmoxtf/`, incl. its ADR corpus vendored at `examples/bpg-provider/docs/adr/` for reference).
- Decision log: 22 confirmation batches (B01–B22) run interactively 2026-09-09; overrides recorded inline in the tables.
