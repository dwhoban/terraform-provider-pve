// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestBackup_Jobs_CRUD covers the backup job wire set: create body with
// property-string groups and joined guest lists, list and read decode with
// boolish flags and grouped objects, update with the delete query, and
// delete.
func TestBackup_Jobs_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/backup":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/backup":
			_, _ = io.WriteString(w, `{"data":[{"id":"daily","enabled":1,"schedule":"mon..fri 02:00","storage":"local","mode":"snapshot","compress":"zstd","vmid":"100,101","next-run":1700000000}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/backup/daily":
			_, _ = io.WriteString(w, `{"data":{"id":"daily","enabled":true,"schedule":"mon..fri 02:00","storage":"local","mode":"snapshot","compress":"zstd","vmid":"100,101","exclude-path":["/tmp/cache"],"prune-backups":{"keep-last":3,"keep-daily":7},"fleecing":{"enabled":1,"storage":"local"},"performance":{"max-workers":2},"bwlimit":5000,"next-run":1700000000}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/backup/daily":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/backup/daily":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	job := BackupJob{
		ID:           "daily",
		Schedule:     "mon..fri 02:00",
		Storage:      "local",
		Mode:         "snapshot",
		Compress:     "zstd",
		VMIDs:        []string{"100", "101"},
		ExcludePath:  []string{"/tmp/cache"},
		BWLimit:      BackupInt64Ptr(5000),
		Fleecing:     &BackupFleecing{Enabled: BackupBoolPtr(true), Storage: "local"},
		Performance:  &BackupPerformance{MaxWorkers: BackupInt64Ptr(2)},
		PruneBackups: &BackupPruneBackups{KeepLast: BackupInt64Ptr(3), KeepDaily: BackupInt64Ptr(7)},
	}
	if err := c.CreateBackupJob(ctx, job); err != nil {
		t.Fatalf("CreateBackupJob: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["id"] != "daily" || sent["schedule"] != "mon..fri 02:00" || sent["bwlimit"] != float64(5000) {
		t.Fatalf("create body = %v", sent)
	}
	if sent["vmid"] != "100,101" {
		t.Fatalf("vmid = %v, want joined list", sent["vmid"])
	}
	excludePath, ok := sent["exclude-path"].([]any)
	if sent["exclude-path"] == nil || !ok || len(excludePath) != 1 {
		t.Fatalf("exclude-path = %v, want JSON array", sent["exclude-path"])
	}
	if sent["fleecing"] != "enabled=1,storage=local" {
		t.Fatalf("fleecing = %v, want property string", sent["fleecing"])
	}
	if sent["performance"] != "max-workers=2" {
		t.Fatalf("performance = %v, want property string", sent["performance"])
	}
	if sent["prune-backups"] != "keep-daily=7,keep-last=3" {
		t.Fatalf("prune-backups = %v, want property string", sent["prune-backups"])
	}

	jobs, err := c.ListBackupJobs(ctx)
	if err != nil {
		t.Fatalf("ListBackupJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "daily" {
		t.Fatalf("jobs = %+v", jobs)
	}
	if jobs[0].Enabled == nil || !*jobs[0].Enabled || jobs[0].NextRun == nil || *jobs[0].NextRun != 1700000000 {
		t.Fatalf("boolish decode = %+v", jobs[0])
	}
	if len(jobs[0].VMIDs) != 2 || jobs[0].VMIDs[1] != "101" {
		t.Fatalf("vmid split = %+v", jobs[0].VMIDs)
	}

	read, err := c.GetBackupJob(ctx, "daily")
	if err != nil {
		t.Fatalf("GetBackupJob: %v", err)
	}
	if read.Compress != "zstd" || len(read.ExcludePath) != 1 || read.ExcludePath[0] != "/tmp/cache" {
		t.Fatalf("read job = %+v", read)
	}
	if read.PruneBackups == nil || read.PruneBackups.KeepLast == nil || *read.PruneBackups.KeepLast != 3 || *read.PruneBackups.KeepDaily != 7 {
		t.Fatalf("prune-backups decode = %+v", read.PruneBackups)
	}
	if read.Fleecing == nil || read.Fleecing.Storage != "local" || read.Fleecing.Enabled == nil || !*read.Fleecing.Enabled {
		t.Fatalf("fleecing decode = %+v", read.Fleecing)
	}
	if read.Performance == nil || read.Performance.MaxWorkers == nil || *read.Performance.MaxWorkers != 2 {
		t.Fatalf("performance decode = %+v", read.Performance)
	}

	update := BackupJob{ID: "daily", Schedule: "sat 03:00", MailTo: "ops@example.com"}
	if err := c.UpdateBackupJob(ctx, "daily", update, []string{"mailnotification", "notes-template"}); err != nil {
		t.Fatalf("UpdateBackupJob: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "mailnotification") || !strings.Contains(lastQuery, "notes-template") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["schedule"] != "sat 03:00" || sent["mailto"] != "ops@example.com" {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteBackupJob(ctx, "daily"); err != nil {
		t.Fatalf("DeleteBackupJob: %v", err)
	}
}

// TestBackup_Job_GroupPropertyStringFallback verifies the GET decode also
// tolerates PVE versions that return the grouped options as property
// strings instead of objects.
func TestBackup_Job_GroupPropertyStringFallback(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/backup/legacy" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"legacy","prune-backups":"keep-last=2,keep-weekly=4","fleecing":"storage=flee","performance":"pbs-entries-max=2048"}}`)
	})
	read, err := c.GetBackupJob(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("GetBackupJob: %v", err)
	}
	if read.PruneBackups == nil || read.PruneBackups.KeepLast == nil || *read.PruneBackups.KeepLast != 2 || read.PruneBackups.KeepWeekly == nil || *read.PruneBackups.KeepWeekly != 4 {
		t.Fatalf("prune-backups = %+v", read.PruneBackups)
	}
	if read.Fleecing == nil || read.Fleecing.Storage != "flee" {
		t.Fatalf("fleecing = %+v", read.Fleecing)
	}
	if read.Performance == nil || read.Performance.PBSEntriesMax == nil || *read.Performance.PBSEntriesMax != 2048 {
		t.Fatalf("performance = %+v", read.Performance)
	}
}

// TestBackup_Job_Delete404IsAPIError confirms a missing job surfaces as a
// 404 *APIError so the resource layer can treat already-absent as success.
func TestBackup_Job_Delete404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/backup/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such job"}`)
	})
	err := c.DeleteBackupJob(context.Background(), "gone")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// TestBackup_NotBackedUp verifies the not-backed-up listing path and
// decode, including the optional guest name.
func TestBackup_NotBackedUp(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/backup-info/not-backed-up" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"vmid":100,"type":"qemu","name":"web"},{"vmid":101,"type":"lxc"}]}`)
	})
	guests, err := c.ListNotBackedUpGuests(context.Background())
	if err != nil {
		t.Fatalf("ListNotBackedUpGuests: %v", err)
	}
	if len(guests) != 2 || guests[0].VMID != 100 || guests[0].Type != "qemu" || guests[0].Name != "web" {
		t.Fatalf("guests = %+v", guests)
	}
	if guests[1].Name != "" || guests[1].Type != "lxc" {
		t.Fatalf("unnamed guest = %+v", guests[1])
	}
}

// TestBackup_IncludedVolumes verifies the per-job included volume tree
// decode.
func TestBackup_IncludedVolumes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/backup/daily/included_volumes" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"children":[{"id":100,"type":"qemu","name":"web","children":[{"id":"scsi0","included":true,"name":"local:100/vm-disk","reason":"included by mode snapshot"},{"id":"efidisk0","included":false,"name":"local:100/vm-efi","reason":"efi disks are skipped"}]},{"id":101,"type":"unknown"}]}}`)
	})
	tree, err := c.GetBackupIncludedVolumes(context.Background(), "daily")
	if err != nil {
		t.Fatalf("GetBackupIncludedVolumes: %v", err)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("children = %+v", tree.Children)
	}
	web := tree.Children[0]
	if web.VMID != 100 || web.Type != "qemu" || web.Name != "web" || len(web.Volumes) != 2 {
		t.Fatalf("guest = %+v", web)
	}
	if web.Volumes[0].ID != "scsi0" || web.Volumes[0].Included == nil || !*web.Volumes[0].Included {
		t.Fatalf("volume = %+v", web.Volumes[0])
	}
	if tree.Children[1].Name != "" || tree.Children[1].Volumes != nil {
		t.Fatalf("unknown guest = %+v", tree.Children[1])
	}
}

// TestBackup_VzdumpDefaults verifies GET /nodes/{node}/vzdump/defaults:
// storage query parameter, boolish flag decode, and the property-string
// group fields the pin declares on this endpoint.
func TestBackup_VzdumpDefaults(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/vzdump/defaults" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("storage") != "local" {
			t.Fatalf("storage query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"all":1,"bwlimit":0,"compress":"zstd","mode":"snapshot","mailnotification":"always","notification-mode":"auto","stdexcludes":0,"remove":1,"stopwait":10,"zstd":1,"exclude-path":["/root"],"fleecing":"enabled=0","performance":"max-workers=16","prune-backups":"keep-all=1","pigz":0,"ionice":7,"lockwait":180}}`)
	})
	defaults, err := c.GetVzdumpDefaults(context.Background(), "pve1", "local")
	if err != nil {
		t.Fatalf("GetVzdumpDefaults: %v", err)
	}
	if defaults.All == nil || !*defaults.All || defaults.StdExcludes == nil || *defaults.StdExcludes {
		t.Fatalf("boolish flags = %+v", defaults)
	}
	if defaults.Compress != "zstd" || defaults.Mode != "snapshot" || defaults.StopWait == nil || *defaults.StopWait != 10 {
		t.Fatalf("defaults = %+v", defaults)
	}
	if len(defaults.ExcludePath) != 1 || defaults.ExcludePath[0] != "/root" {
		t.Fatalf("exclude-path = %+v", defaults.ExcludePath)
	}
	if defaults.Fleecing != "enabled=0" || defaults.Performance != "max-workers=16" || defaults.PruneBackups != "keep-all=1" {
		t.Fatalf("property strings = %q %q %q", defaults.Fleecing, defaults.Performance, defaults.PruneBackups)
	}
}
