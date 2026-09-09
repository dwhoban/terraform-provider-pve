// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// BackupBoolPtr returns a pointer to b, for building backup job structs.
func BackupBoolPtr(b bool) *bool { return &b }

// BackupInt64Ptr returns a pointer to i, for building backup job structs.
func BackupInt64Ptr(i int64) *int64 { return &i }

// BackupJob is a vzdump backup job (GET/POST /cluster/backup,
// GET/PUT/DELETE /cluster/backup/{id}). The guest lists (VMIDs, Exclude)
// travel on the wire as comma-separated strings and ExcludePath as a JSON
// array. The grouped options Fleecing, Performance, and PruneBackups are
// objects on GET but property strings on POST/PUT; the decode also
// tolerates PVE versions that return them as property strings. Boolean
// fields tolerate the 0/1 int encoding PVE emits on some versions.
type BackupJob struct {
	ID                     string
	Node                   string
	Comment                string
	Schedule               string
	Enabled                *bool
	Mode                   string
	Compress               string
	Storage                string
	DumpDir                string
	TmpDir                 string
	Script                 string
	All                    *bool
	VMIDs                  []string
	Exclude                []string
	ExcludePath            []string
	Pool                   string
	NotesTemplate          string
	MailTo                 string
	MailNotification       string
	NotificationMode       string
	PBSChangeDetectionMode string
	BWLimit                *int64
	Zstd                   *int64
	Pigz                   *int64
	IONice                 *int64
	LockWait               *int64
	StopWait               *int64
	StdExcludes            *bool
	Quiet                  *bool
	Stop                   *bool
	Remove                 *bool
	RepeatMissed           *bool
	Protected              *bool
	Fleecing               *BackupFleecing
	Performance            *BackupPerformance
	PruneBackups           *BackupPruneBackups
	NextRun                *int64
}

// BackupFleecing holds the backup fleecing options (VM only).
type BackupFleecing struct {
	Enabled *bool
	Storage string
}

// BackupPerformance holds the performance-related settings.
type BackupPerformance struct {
	MaxWorkers    *int64
	PBSEntriesMax *int64
}

// BackupPruneBackups holds the retention options that override the
// storage configuration.
type BackupPruneBackups struct {
	KeepAll     *bool
	KeepHourly  *int64
	KeepDaily   *int64
	KeepWeekly  *int64
	KeepMonthly *int64
	KeepYearly  *int64
	KeepLast    *int64
}

// backupJobWire mirrors the wire shape of BackupJob with hyphenated keys,
// joined guest lists, and lenient fields as json.RawMessage.
type backupJobWire struct {
	ID                     string          `json:"id,omitempty"`
	Node                   string          `json:"node,omitempty"`
	Comment                string          `json:"comment,omitempty"`
	Schedule               string          `json:"schedule,omitempty"`
	Enabled                json.RawMessage `json:"enabled,omitempty"`
	Mode                   string          `json:"mode,omitempty"`
	Compress               string          `json:"compress,omitempty"`
	Storage                string          `json:"storage,omitempty"`
	DumpDir                string          `json:"dumpdir,omitempty"`
	TmpDir                 string          `json:"tmpdir,omitempty"`
	Script                 string          `json:"script,omitempty"`
	All                    json.RawMessage `json:"all,omitempty"`
	VMIDs                  string          `json:"vmid,omitempty"`
	Exclude                string          `json:"exclude,omitempty"`
	ExcludePath            []string        `json:"exclude-path,omitempty"`
	Pool                   string          `json:"pool,omitempty"`
	NotesTemplate          string          `json:"notes-template,omitempty"`
	MailTo                 string          `json:"mailto,omitempty"`
	MailNotification       string          `json:"mailnotification,omitempty"`
	NotificationMode       string          `json:"notification-mode,omitempty"`
	PBSChangeDetectionMode string          `json:"pbs-change-detection-mode,omitempty"`
	BWLimit                *int64          `json:"bwlimit,omitempty"`
	Zstd                   *int64          `json:"zstd,omitempty"`
	Pigz                   *int64          `json:"pigz,omitempty"`
	IONice                 *int64          `json:"ionice,omitempty"`
	LockWait               *int64          `json:"lockwait,omitempty"`
	StopWait               *int64          `json:"stopwait,omitempty"`
	StdExcludes            json.RawMessage `json:"stdexcludes,omitempty"`
	Quiet                  json.RawMessage `json:"quiet,omitempty"`
	Stop                   json.RawMessage `json:"stop,omitempty"`
	Remove                 json.RawMessage `json:"remove,omitempty"`
	RepeatMissed           json.RawMessage `json:"repeat-missed,omitempty"`
	Protected              json.RawMessage `json:"protected,omitempty"`
	Fleecing               json.RawMessage `json:"fleecing,omitempty"`
	Performance            json.RawMessage `json:"performance,omitempty"`
	PruneBackups           json.RawMessage `json:"prune-backups,omitempty"`
	NextRun                *int64          `json:"next-run,omitempty"`
}

// MarshalJSON encodes the guest lists as PVE's comma-separated wire
// strings and the grouped options as property strings.
func (j BackupJob) MarshalJSON() ([]byte, error) {
	w := backupJobWire{
		ID:                     j.ID,
		Node:                   j.Node,
		Comment:                j.Comment,
		Schedule:               j.Schedule,
		Mode:                   j.Mode,
		Compress:               j.Compress,
		Storage:                j.Storage,
		DumpDir:                j.DumpDir,
		TmpDir:                 j.TmpDir,
		Script:                 j.Script,
		VMIDs:                  strings.Join(j.VMIDs, ","),
		Exclude:                strings.Join(j.Exclude, ","),
		ExcludePath:            j.ExcludePath,
		Pool:                   j.Pool,
		NotesTemplate:          j.NotesTemplate,
		MailTo:                 j.MailTo,
		MailNotification:       j.MailNotification,
		NotificationMode:       j.NotificationMode,
		PBSChangeDetectionMode: j.PBSChangeDetectionMode,
		BWLimit:                j.BWLimit,
		Zstd:                   j.Zstd,
		Pigz:                   j.Pigz,
		IONice:                 j.IONice,
		LockWait:               j.LockWait,
		StopWait:               j.StopWait,
		NextRun:                j.NextRun,
	}
	w.Enabled = backupBoolRaw(j.Enabled)
	w.All = backupBoolRaw(j.All)
	w.StdExcludes = backupBoolRaw(j.StdExcludes)
	w.Quiet = backupBoolRaw(j.Quiet)
	w.Stop = backupBoolRaw(j.Stop)
	w.Remove = backupBoolRaw(j.Remove)
	w.RepeatMissed = backupBoolRaw(j.RepeatMissed)
	w.Protected = backupBoolRaw(j.Protected)
	if j.Fleecing != nil {
		w.Fleecing = backupJSONString(backupFleecingString(j.Fleecing))
	}
	if j.Performance != nil {
		w.Performance = backupJSONString(backupPerformanceString(j.Performance))
	}
	if j.PruneBackups != nil {
		w.PruneBackups = backupJSONString(backupPruneBackupsString(j.PruneBackups))
	}
	return json.Marshal(w)
}

// UnmarshalJSON splits the wire guest lists, decodes the boolish flags,
// and decodes the grouped options as objects or property strings.
func (j *BackupJob) UnmarshalJSON(data []byte) error {
	var w backupJobWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	j.ID = w.ID
	j.Node = w.Node
	j.Comment = w.Comment
	j.Schedule = w.Schedule
	j.Mode = w.Mode
	j.Compress = w.Compress
	j.Storage = w.Storage
	j.DumpDir = w.DumpDir
	j.TmpDir = w.TmpDir
	j.Script = w.Script
	j.VMIDs = haSplitList(w.VMIDs)
	j.Exclude = haSplitList(w.Exclude)
	j.ExcludePath = w.ExcludePath
	j.Pool = w.Pool
	j.NotesTemplate = w.NotesTemplate
	j.MailTo = w.MailTo
	j.MailNotification = w.MailNotification
	j.NotificationMode = w.NotificationMode
	j.PBSChangeDetectionMode = w.PBSChangeDetectionMode
	j.BWLimit = w.BWLimit
	j.Zstd = w.Zstd
	j.Pigz = w.Pigz
	j.IONice = w.IONice
	j.LockWait = w.LockWait
	j.StopWait = w.StopWait
	j.NextRun = w.NextRun
	j.Enabled = nodeNetworkBoolishPtr(w.Enabled)
	j.All = nodeNetworkBoolishPtr(w.All)
	j.StdExcludes = nodeNetworkBoolishPtr(w.StdExcludes)
	j.Quiet = nodeNetworkBoolishPtr(w.Quiet)
	j.Stop = nodeNetworkBoolishPtr(w.Stop)
	j.Remove = nodeNetworkBoolishPtr(w.Remove)
	j.RepeatMissed = nodeNetworkBoolishPtr(w.RepeatMissed)
	j.Protected = nodeNetworkBoolishPtr(w.Protected)
	var err error
	if j.Fleecing, err = backupFleecingFromRaw(w.Fleecing); err != nil {
		return fmt.Errorf("decoding fleecing: %w", err)
	}
	if j.Performance, err = backupPerformanceFromRaw(w.Performance); err != nil {
		return fmt.Errorf("decoding performance: %w", err)
	}
	if j.PruneBackups, err = backupPruneBackupsFromRaw(w.PruneBackups); err != nil {
		return fmt.Errorf("decoding prune-backups: %w", err)
	}
	return nil
}

// backupBoolRaw encodes an optional bool as a raw JSON value, nil for nil.
func backupBoolRaw(b *bool) json.RawMessage {
	if b == nil {
		return nil
	}
	return haBoolRaw(*b)
}

// backupJSONString encodes s as a raw JSON string value.
func backupJSONString(s string) json.RawMessage {
	out, _ := json.Marshal(s)
	return out
}

// backupFleecingString renders the fleecing property string.
func backupFleecingString(f *BackupFleecing) string {
	var parts []string
	if f.Enabled != nil {
		parts = append(parts, "enabled="+backupBool01(*f.Enabled))
	}
	if f.Storage != "" {
		parts = append(parts, "storage="+f.Storage)
	}
	return strings.Join(parts, ",")
}

// backupPerformanceString renders the performance property string.
func backupPerformanceString(p *BackupPerformance) string {
	var parts []string
	if p.MaxWorkers != nil {
		parts = append(parts, "max-workers="+strconv.FormatInt(*p.MaxWorkers, 10))
	}
	if p.PBSEntriesMax != nil {
		parts = append(parts, "pbs-entries-max="+strconv.FormatInt(*p.PBSEntriesMax, 10))
	}
	return strings.Join(parts, ",")
}

// backupPruneBackupsString renders the prune-backups property string.
func backupPruneBackupsString(p *BackupPruneBackups) string {
	var parts []string
	if p.KeepAll != nil {
		parts = append(parts, "keep-all="+backupBool01(*p.KeepAll))
	}
	for _, e := range []struct {
		key string
		val *int64
	}{
		{"keep-daily", p.KeepDaily},
		{"keep-hourly", p.KeepHourly},
		{"keep-last", p.KeepLast},
		{"keep-monthly", p.KeepMonthly},
		{"keep-weekly", p.KeepWeekly},
		{"keep-yearly", p.KeepYearly},
	} {
		if e.val != nil {
			parts = append(parts, e.key+"="+strconv.FormatInt(*e.val, 10))
		}
	}
	return strings.Join(parts, ",")
}

// backupBool01 renders b as PVE's 1/0 property-string encoding.
func backupBool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// backupFleecingFromRaw decodes the fleecing group from an object or a
// property string, returning nil for absent values.
func backupFleecingFromRaw(raw json.RawMessage) (*BackupFleecing, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		var w struct {
			Enabled json.RawMessage `json:"enabled,omitempty"`
			Storage string          `json:"storage,omitempty"`
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, err
		}
		return &BackupFleecing{Enabled: nodeNetworkBoolishPtr(w.Enabled), Storage: w.Storage}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unexpected payload %s", raw)
	}
	kv, err := clusterOptionsParseKV("fleecing", s)
	if err != nil {
		return nil, err
	}
	storage := clusterOptionsKVStr(kv, "storage")
	out := &BackupFleecing{}
	if storage != nil {
		out.Storage = *storage
	}
	if enabled, err := clusterOptionsKVBool("fleecing", "enabled", kv); err != nil {
		return nil, err
	} else if enabled != nil {
		out.Enabled = enabled
	}
	return out, nil
}

// backupPerformanceFromRaw decodes the performance group from an object or
// a property string, returning nil for absent values.
func backupPerformanceFromRaw(raw json.RawMessage) (*BackupPerformance, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		var w struct {
			MaxWorkers    *int64 `json:"max-workers,omitempty"`
			PBSEntriesMax *int64 `json:"pbs-entries-max,omitempty"`
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, err
		}
		return &BackupPerformance{MaxWorkers: w.MaxWorkers, PBSEntriesMax: w.PBSEntriesMax}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unexpected payload %s", raw)
	}
	kv, err := clusterOptionsParseKV("performance", s)
	if err != nil {
		return nil, err
	}
	maxWorkers, err := clusterOptionsKVInt("performance", "max-workers", kv)
	if err != nil {
		return nil, err
	}
	pbsEntriesMax, err := clusterOptionsKVInt("performance", "pbs-entries-max", kv)
	if err != nil {
		return nil, err
	}
	return &BackupPerformance{MaxWorkers: backupInt64FromInt(maxWorkers), PBSEntriesMax: backupInt64FromInt(pbsEntriesMax)}, nil
}

// backupPruneBackupsFromRaw decodes the prune-backups group from an object
// or a property string, returning nil for absent values.
func backupPruneBackupsFromRaw(raw json.RawMessage) (*BackupPruneBackups, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		var w struct {
			KeepAll     json.RawMessage `json:"keep-all,omitempty"`
			KeepHourly  *int64          `json:"keep-hourly,omitempty"`
			KeepDaily   *int64          `json:"keep-daily,omitempty"`
			KeepWeekly  *int64          `json:"keep-weekly,omitempty"`
			KeepMonthly *int64          `json:"keep-monthly,omitempty"`
			KeepYearly  *int64          `json:"keep-yearly,omitempty"`
			KeepLast    *int64          `json:"keep-last,omitempty"`
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, err
		}
		return &BackupPruneBackups{
			KeepAll:     nodeNetworkBoolishPtr(w.KeepAll),
			KeepHourly:  w.KeepHourly,
			KeepDaily:   w.KeepDaily,
			KeepWeekly:  w.KeepWeekly,
			KeepMonthly: w.KeepMonthly,
			KeepYearly:  w.KeepYearly,
			KeepLast:    w.KeepLast,
		}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unexpected payload %s", raw)
	}
	kv, err := clusterOptionsParseKV("prune-backups", s)
	if err != nil {
		return nil, err
	}
	out := &BackupPruneBackups{}
	keepAll, err := clusterOptionsKVBool("prune-backups", "keep-all", kv)
	if err != nil {
		return nil, err
	}
	out.KeepAll = keepAll
	for _, e := range []struct {
		key string
		dst **int64
	}{
		{"keep-hourly", &out.KeepHourly},
		{"keep-daily", &out.KeepDaily},
		{"keep-weekly", &out.KeepWeekly},
		{"keep-monthly", &out.KeepMonthly},
		{"keep-yearly", &out.KeepYearly},
		{"keep-last", &out.KeepLast},
	} {
		v, err := clusterOptionsKVInt("prune-backups", e.key, kv)
		if err != nil {
			return nil, err
		}
		*e.dst = backupInt64FromInt(v)
	}
	return out, nil
}

// backupInt64FromInt converts the clusterOptionsKVInt helper's *int into
// an *int64, keeping nil for absent values.
func backupInt64FromInt(v *int) *int64 {
	if v == nil {
		return nil
	}
	out := int64(*v)
	return &out
}

// BackupNotBackedUpGuest is one entry of GET
// /cluster/backup-info/not-backed-up: a guest not covered by any backup
// job.
type BackupNotBackedUpGuest struct {
	VMID int64  `json:"vmid"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// BackupIncludedVolumes is the root of the per-job included volume tree
// (GET /cluster/backup/{id}/included_volumes). Children represent guests;
// each guest's volumes carry their inclusion decision and reason.
type BackupIncludedVolumes struct {
	Children []BackupIncludedGuest `json:"children"`
}

// BackupIncludedGuest is one guest in the included volume tree.
type BackupIncludedGuest struct {
	VMID    int64                  `json:"id"`
	Type    string                 `json:"type"`
	Name    string                 `json:"name,omitempty"`
	Volumes []BackupIncludedVolume `json:"-"`
}

// BackupIncludedVolume is one volume of a guest in the included tree.
type BackupIncludedVolume struct {
	ID       string `json:"id"`
	Included *bool  `json:"-"`
	Name     string `json:"name,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// backupIncludedGuestWire mirrors the wire shape of a guest entry.
type backupIncludedGuestWire struct {
	VMID     int64                      `json:"id"`
	Type     string                     `json:"type"`
	Name     string                     `json:"name,omitempty"`
	Children []backupIncludedVolumeWire `json:"children,omitempty"`
}

// backupIncludedVolumeWire mirrors the wire shape of a volume entry.
type backupIncludedVolumeWire struct {
	ID       string          `json:"id"`
	Included json.RawMessage `json:"included,omitempty"`
	Name     string          `json:"name,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// UnmarshalJSON decodes the nested volume list and its boolish included
// flags.
func (g *BackupIncludedGuest) UnmarshalJSON(data []byte) error {
	var w backupIncludedGuestWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	g.VMID = w.VMID
	g.Type = w.Type
	g.Name = w.Name
	if len(w.Children) > 0 {
		g.Volumes = make([]BackupIncludedVolume, 0, len(w.Children))
		for _, v := range w.Children {
			g.Volumes = append(g.Volumes, BackupIncludedVolume{
				ID:       v.ID,
				Included: nodeNetworkBoolishPtr(v.Included),
				Name:     v.Name,
				Reason:   v.Reason,
			})
		}
	}
	return nil
}

// VzdumpDefaults mirrors GET /nodes/{node}/vzdump/defaults. The grouped
// options arrive as property strings on this endpoint and are surfaced
// verbatim; boolean fields tolerate the 0/1 int encoding.
type VzdumpDefaults struct {
	All                    *bool
	BWLimit                *int64
	Compress               string
	DumpDir                string
	Exclude                string
	ExcludePath            []string
	Fleecing               string
	IONice                 *int64
	LockWait               *int64
	MailNotification       string
	MailTo                 string
	Mode                   string
	Node                   string
	NotesTemplate          string
	NotificationMode       string
	PBSChangeDetectionMode string
	Performance            string
	Pigz                   *int64
	Pool                   string
	Protected              *bool
	PruneBackups           string
	Quiet                  *bool
	Remove                 *bool
	Script                 string
	StdExcludes            *bool
	Stop                   *bool
	StopWait               *int64
	Storage                string
	TmpDir                 string
	VMID                   string
	Zstd                   *int64
}

// vzdumpDefaultsWire mirrors the wire shape of VzdumpDefaults.
type vzdumpDefaultsWire struct {
	All                    json.RawMessage `json:"all,omitempty"`
	BWLimit                *int64          `json:"bwlimit,omitempty"`
	Compress               string          `json:"compress,omitempty"`
	DumpDir                string          `json:"dumpdir,omitempty"`
	Exclude                string          `json:"exclude,omitempty"`
	ExcludePath            []string        `json:"exclude-path,omitempty"`
	Fleecing               string          `json:"fleecing,omitempty"`
	IONice                 *int64          `json:"ionice,omitempty"`
	LockWait               *int64          `json:"lockwait,omitempty"`
	MailNotification       string          `json:"mailnotification,omitempty"`
	MailTo                 string          `json:"mailto,omitempty"`
	Mode                   string          `json:"mode,omitempty"`
	Node                   string          `json:"node,omitempty"`
	NotesTemplate          string          `json:"notes-template,omitempty"`
	NotificationMode       string          `json:"notification-mode,omitempty"`
	PBSChangeDetectionMode string          `json:"pbs-change-detection-mode,omitempty"`
	Performance            string          `json:"performance,omitempty"`
	Pigz                   *int64          `json:"pigz,omitempty"`
	Pool                   string          `json:"pool,omitempty"`
	Protected              json.RawMessage `json:"protected,omitempty"`
	PruneBackups           string          `json:"prune-backups,omitempty"`
	Quiet                  json.RawMessage `json:"quiet,omitempty"`
	Remove                 json.RawMessage `json:"remove,omitempty"`
	Script                 string          `json:"script,omitempty"`
	StdExcludes            json.RawMessage `json:"stdexcludes,omitempty"`
	Stop                   json.RawMessage `json:"stop,omitempty"`
	StopWait               *int64          `json:"stopwait,omitempty"`
	Storage                string          `json:"storage,omitempty"`
	TmpDir                 string          `json:"tmpdir,omitempty"`
	VMID                   string          `json:"vmid,omitempty"`
	Zstd                   *int64          `json:"zstd,omitempty"`
}

// UnmarshalJSON decodes the boolish flags.
func (d *VzdumpDefaults) UnmarshalJSON(data []byte) error {
	var w vzdumpDefaultsWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	d.All = nodeNetworkBoolishPtr(w.All)
	d.BWLimit = w.BWLimit
	d.Compress = w.Compress
	d.DumpDir = w.DumpDir
	d.Exclude = w.Exclude
	d.ExcludePath = w.ExcludePath
	d.Fleecing = w.Fleecing
	d.IONice = w.IONice
	d.LockWait = w.LockWait
	d.MailNotification = w.MailNotification
	d.MailTo = w.MailTo
	d.Mode = w.Mode
	d.Node = w.Node
	d.NotesTemplate = w.NotesTemplate
	d.NotificationMode = w.NotificationMode
	d.PBSChangeDetectionMode = w.PBSChangeDetectionMode
	d.Performance = w.Performance
	d.Pigz = w.Pigz
	d.Pool = w.Pool
	d.Protected = nodeNetworkBoolishPtr(w.Protected)
	d.PruneBackups = w.PruneBackups
	d.Quiet = nodeNetworkBoolishPtr(w.Quiet)
	d.Remove = nodeNetworkBoolishPtr(w.Remove)
	d.Script = w.Script
	d.StdExcludes = nodeNetworkBoolishPtr(w.StdExcludes)
	d.Stop = nodeNetworkBoolishPtr(w.Stop)
	d.StopWait = w.StopWait
	d.Storage = w.Storage
	d.TmpDir = w.TmpDir
	d.VMID = w.VMID
	d.Zstd = w.Zstd
	return nil
}

// ListBackupJobs returns the jobs from GET /cluster/backup.
func (c *Client) ListBackupJobs(ctx context.Context) ([]BackupJob, error) {
	var out []BackupJob
	if err := c.Do(ctx, "GET", "/cluster/backup", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBackupJob reads /cluster/backup/{id}.
func (c *Client) GetBackupJob(ctx context.Context, id string) (*BackupJob, error) {
	var out BackupJob
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/backup/%s", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateBackupJob POSTs /cluster/backup with the supplied job. The call is
// synchronous per the pin (returns null, no task).
func (c *Client) CreateBackupJob(ctx context.Context, job BackupJob) error {
	return c.Do(ctx, "POST", "/cluster/backup", job, nil)
}

// UpdateBackupJob PUTs /cluster/backup/{id} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter. The
// call is synchronous per the pin.
func (c *Client) UpdateBackupJob(ctx context.Context, id string, job BackupJob, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/backup/%s", id), deleteFields)
	return c.Do(ctx, "PUT", path, job, nil)
}

// DeleteBackupJob DELETEs /cluster/backup/{id}. The call is synchronous
// per the pin.
func (c *Client) DeleteBackupJob(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/backup/%s", id), nil, nil)
}

// ListNotBackedUpGuests returns the guests not covered by any backup job
// from GET /cluster/backup-info/not-backed-up.
func (c *Client) ListNotBackedUpGuests(ctx context.Context) ([]BackupNotBackedUpGuest, error) {
	var out []BackupNotBackedUpGuest
	if err := c.Do(ctx, "GET", "/cluster/backup-info/not-backed-up", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBackupIncludedVolumes returns the per-job included volume tree from
// GET /cluster/backup/{id}/included_volumes.
func (c *Client) GetBackupIncludedVolumes(ctx context.Context, id string) (*BackupIncludedVolumes, error) {
	var out BackupIncludedVolumes
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/backup/%s/included_volumes", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVzdumpDefaults reads GET /nodes/{node}/vzdump/defaults; storage
// optionally scopes the defaults to one storage's configuration.
func (c *Client) GetVzdumpDefaults(ctx context.Context, node, storage string) (*VzdumpDefaults, error) {
	path := fmt.Sprintf("/nodes/%s/vzdump/defaults", node)
	if storage != "" {
		path += "?storage=" + url.QueryEscape(storage)
	}
	var out VzdumpDefaults
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
