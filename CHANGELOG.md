## 0.1.0 (Unreleased)

- Provider identity `registry.terraform.io/hashicorp/pve` (Terraform Plugin Framework, protocol 6), with API-token and username/password credential chains, TLS controls, and `PROXMOX_VE_*` environment resolution
- Full ADR-0001 API surface: 87 managed resources, 106 data sources, 41 actions, and the `next_id` provider function, pinned by `TestProvider_RegisteredSurface`
- Access control: users, tokens, groups, roles, ACLs, permissions, LDAP/AD/OpenID realms, realm sync jobs
- Cluster: membership, options, resources/status/tasks reads, HA groups/resources/rules/status/arm and move actions
- Firewall: cluster/node/guest/vnet options singletons, aliases, ipsets, security groups, and five ordered rule-list resources
- Backup and replication: backup jobs (with not-backed-up and vzdump-defaults reads), replication jobs and per-node status, prune-backups action
- Metrics and notifications: metric servers, four notification endpoint types, matchers, targets, and test action
- Storage: eleven typed storages, node/storage listings, ISO/template file upload and URL download, prune action (`glusterfs` omitted: absent from the pinned API spec)
- Nodes: hosts file, services, apt repositories and changelogs, tasks, PCI/USB inventory, capabilities, subscriptions, certificates, wake-on-LAN, bulk power actions, and the restricted `node_execute` action
- Disks and Ceph: ZFS, LVM, LVM-thin, directory disks, initgpt action, Ceph status/pools/OSDs/monitors
- Guests: `pve_vm` and `pve_container` with clone support, growth-only disk resize, live migration on node change, cloud-init, snapshots with rollback, and per-guest power/migrate actions
- SDN: five zone types, vnets, subnets, controllers, DNS, IPAMs, prefix lists, route maps, OSPF/OpenFabric fabrics, apply/rollback actions
- ACME: accounts, DNS plugins, certificates; hardware mappings (PCI/USB), mapping directories, custom CPU models
- Vendored API pin (`api-spec/apidoc.js`) as the authoritative schema source, with generated Registry documentation for every component
