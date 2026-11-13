// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase5 — storage & pools: cluster storage configuration, resource pools,
// and storage content (files/volumes) with their one-shot operations.
var Phase5 = []Type{
	{
		Type: "pve_storage", Kind: "resource", Phase: 5,
		Summary: "Cluster storage configuration (any backend: dir, nfs, cifs, zfs, lvm, ceph, pbs, …).",
		Create:  &Endpoint{Path: "/storage"},
		Read:    &Endpoint{Path: "/storage/{storage}"},
		Update:  &Endpoint{Path: "/storage/{storage}"},
		Delete:  &Endpoint{Path: "/storage/{storage}"},
		Attrs: []AttrRule{
			{Name: "storage", ForceNew: true},
			// PVE stores content lists as append sets in storage.cfg:
			// model as a string set so equality is order-insensitive.
			{Name: "content", Type: "set", Elem: "string"},
			// digest is a per-write conflict token: it changes on every
			// write and has no declarative meaning.
			{Name: "digest", Skip: true},
		},
	},
	{
		Type: "pve_storage", Kind: "data_source", Phase: 5,
		Summary: "All cluster storages; optionally the content overview of one storage on one node.",
		Reads: []*Endpoint{
			{Path: "/storage", ResultName: "storages"},
			{Path: "/nodes/{node}/storage/{storage}/content", ResultName: "content", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "node", Required: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "storage", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_pool", Kind: "resource", Phase: 5,
		Summary: "Resource pool (grouping of guests and storages; members are computed).",
		Create:  &Endpoint{Path: "/pools"},
		Read:    &Endpoint{Path: "/pools/{poolid}"},
		Update:  &Endpoint{Path: "/pools/{poolid}"},
		Delete:  &Endpoint{Path: "/pools/{poolid}"},
		Attrs: []AttrRule{
			{Name: "poolid", ForceNew: true},
		},
	},
	{
		Type: "pve_file", Kind: "resource", Phase: 5,
		Summary: "Storage content volume/file: created empty or by name on a node's storage, attributes updated, deleted by volume id.",
		Create:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/content"},
		Read:    &Endpoint{Path: "/nodes/{node}/storage/{storage}/content/{volume}"},
		Update:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/content/{volume}"},
		Delete:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/content/{volume}"},
		Attrs: []AttrRule{
			// Create returns the new volume identifier (volid) as a plain
			// string; fillAllocatedID stores it in `volume`.
			{Name: "volume", Required: BoolPtr(false), Optional: BoolPtr(true), Computed: BoolPtr(true), ForceNew: true},
		},
	},

	// Storage content one-shot operations.
	{
		Type: "pve_file_upload", Kind: "action", Phase: 5,
		Summary: "Upload a file to a storage (checksum-verified).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/upload"},
	},
	{
		Type: "pve_file_download_url", Kind: "action", Phase: 5,
		Summary: "Download a URL into a storage file.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/download-url"},
	},
	{
		Type: "pve_file_oci_pull", Kind: "action", Phase: 5,
		Summary: "Pull an OCI image artifact into a storage template volume.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/oci-registry-pull"},
	},
	{
		Type: "pve_storage_prune_backups", Kind: "action", Phase: 5,
		Summary: "Prune backups on a storage according to retention options.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/prunebackups", Method: "DELETE"},
	},
	{
		Type: "pve_storage_prune_backups_preview", Kind: "data_source", Phase: 5,
		Summary: "Backups that pruning would remove from a storage (dry run).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/storage/{storage}/prunebackups", ResultName: "backups"}},
	},

	// Storage introspection data sources.
	{
		Type: "pve_storage_identity", Kind: "data_source", Phase: 5,
		Summary: "Identity of a storage backend instance (unique id, sysfs/cluster identity info).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/storage/{storage}/identity", ResultName: "identity"}},
	},
	{
		Type: "pve_storage_import_metadata", Kind: "data_source", Phase: 5,
		Summary: "Import metadata for a volume (how to import it as a guest).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/storage/{storage}/import-metadata", ResultName: "metadata"}},
	},
	{
		Type: "pve_filerestore_list", Kind: "data_source", Phase: 5,
		Summary: "File-restore listing of a backup volume (optionally under one path).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/storage/{storage}/file-restore/list", ResultName: "entries"}},
	},
	{
		Type: "pve_filerestore_download", Kind: "action", Phase: 5,
		Summary: "Download one file from a backup volume via file-restore.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/storage/{storage}/file-restore/download", Method: "GET"},
	},
}
