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

// ReplicationJob is one storage replication job configuration as managed
// through /cluster/replication. The ID is composed of a Guest ID and a job
// number separated by a hyphen (`<GUEST>-<JOBNUM>`, e.g. `100-0`), which is
// how PVE binds the job to its guest; Target is not updatable through the
// pin's PUT and therefore only settable at create time. Rate limits the
// sync in MB/s. Disable tolerates the 0/1 int encoding PVE emits.
type ReplicationJob struct {
	ID       string   `json:"id"`
	Type     string   `json:"type,omitempty"`
	Target   string   `json:"target"`
	Guest    int64    `json:"guest,omitempty"`
	JobNum   int64    `json:"jobnum,omitempty"`
	Schedule string   `json:"schedule,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Comment  string   `json:"comment,omitempty"`
	Disable  bool     `json:"disable,omitempty"`
	Digest   string   `json:"digest,omitempty"`
}

// replicationJobRaw shadows ReplicationJob for wire decoding, keeping the
// disable flag raw so the lenient decodeBoolish can parse it.
type replicationJobRaw struct {
	ID       string          `json:"id"`
	Type     string          `json:"type,omitempty"`
	Target   string          `json:"target"`
	Guest    int64           `json:"guest,omitempty"`
	JobNum   int64           `json:"jobnum,omitempty"`
	Schedule string          `json:"schedule,omitempty"`
	Rate     *float64        `json:"rate,omitempty"`
	Comment  string          `json:"comment,omitempty"`
	Disable  json.RawMessage `json:"disable,omitempty"`
	Digest   string          `json:"digest,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of disable that PVE emits.
func (j *ReplicationJob) UnmarshalJSON(data []byte) error {
	var raw replicationJobRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	j.ID = raw.ID
	j.Type = raw.Type
	j.Target = raw.Target
	j.Guest = raw.Guest
	j.JobNum = raw.JobNum
	j.Schedule = raw.Schedule
	j.Rate = raw.Rate
	j.Comment = raw.Comment
	j.Disable = decodeBoolish(raw.Disable)
	j.Digest = raw.Digest
	return nil
}

// ListReplications enumerates GET /cluster/replication.
func (c *Client) ListReplications(ctx context.Context) ([]ReplicationJob, error) {
	var out []ReplicationJob
	if err := c.Do(ctx, "GET", "/cluster/replication", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetReplication reads GET /cluster/replication/{id}.
func (c *Client) GetReplication(ctx context.Context, id string) (*ReplicationJob, error) {
	var out ReplicationJob
	if err := c.Do(ctx, "GET", "/cluster/replication/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// replicationJobCreateBody is the JSON body of POST /cluster/replication.
// The pin marks the section type required for create; this client always
// sends the only legal value, "local".
type replicationJobCreateBody struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Target   string   `json:"target"`
	Schedule string   `json:"schedule,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Comment  string   `json:"comment,omitempty"`
	Disable  bool     `json:"disable,omitempty"`
}

// CreateReplication POSTs /cluster/replication. Absent optional fields are
// omitted so PVE applies its own defaults (schedule defaults to `*/15`).
func (c *Client) CreateReplication(ctx context.Context, job ReplicationJob) error {
	body := replicationJobCreateBody{
		ID:       job.ID,
		Type:     "local",
		Target:   job.Target,
		Schedule: job.Schedule,
		Rate:     job.Rate,
		Comment:  job.Comment,
		Disable:  job.Disable,
	}
	return c.Do(ctx, "POST", "/cluster/replication", body, nil)
}

// ReplicationJobUpdate carries the fields of PUT /cluster/replication/{id}.
// Settings listed in Delete travel in the PVE `delete` parameter (a
// comma-separated name list); Disable false clears through Delete rather
// than a false body value, matching how PVE stores only truthy flags.
type ReplicationJobUpdate struct {
	Schedule string
	Rate     *float64
	Comment  string
	Disable  bool
	Delete   []string
}

// replicationJobUpdateBody is the JSON body of PUT /cluster/replication/{id}.
// Every settable field is omitempty: absent fields stay untouched upstream,
// and cleared settings travel exclusively via the delete name list.
type replicationJobUpdateBody struct {
	ID       string   `json:"id"`
	Schedule string   `json:"schedule,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Comment  string   `json:"comment,omitempty"`
	Disable  bool     `json:"disable,omitempty"`
	Delete   string   `json:"delete,omitempty"`
}

// UpdateReplication PUTs /cluster/replication/{id}.
func (c *Client) UpdateReplication(ctx context.Context, id string, upd ReplicationJobUpdate) error {
	body := replicationJobUpdateBody{
		ID:       id,
		Schedule: upd.Schedule,
		Rate:     upd.Rate,
		Comment:  upd.Comment,
		Disable:  upd.Disable,
		Delete:   strings.Join(upd.Delete, ","),
	}
	return c.Do(ctx, "PUT", "/cluster/replication/"+url.PathEscape(id), body, nil)
}

// DeleteReplication DELETEs /cluster/replication/{id}, which marks the job
// for removal; PVE's replication worker then cleans up asynchronously.
func (c *Client) DeleteReplication(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", "/cluster/replication/"+url.PathEscape(id), nil, nil)
}

// ReplicationJobStatus is one row of GET /nodes/{node}/replication: the
// per-node runtime view of a replication job with its sync state. The pin
// types the response openly (only id is named), so the struct mirrors the
// fields PVE emits in practice; absent fields decode as zero values and
// Rate as a nil pointer when PVE sends null. Disable and Removal tolerate
// the 0/1 int encoding.
type ReplicationJobStatus struct {
	ID        string   `json:"id"`
	Type      string   `json:"type,omitempty"`
	Target    string   `json:"target,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Disable   bool     `json:"disable,omitempty"`
	Guest     int64    `json:"guest,omitempty"`
	GuestName string   `json:"guest_name,omitempty"`
	JobNum    int64    `json:"jobnum,omitempty"`
	Schedule  string   `json:"schedule,omitempty"`
	Rate      *float64 `json:"rate,omitempty"`
	Removal   bool     `json:"removal,omitempty"`
	LastSync  int64    `json:"last_sync,omitempty"`
	LastTry   int64    `json:"last_try,omitempty"`
	NextSync  int64    `json:"next_sync,omitempty"`
	FailCount int64    `json:"fail_count,omitempty"`
	Duration  int64    `json:"duration,omitempty"`
	Status    string   `json:"status,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// replicationJobStatusRaw shadows ReplicationJobStatus for wire decoding,
// keeping the disable and removal flags raw for lenient decoding.
type replicationJobStatusRaw struct {
	ID        string          `json:"id"`
	Type      string          `json:"type,omitempty"`
	Target    string          `json:"target,omitempty"`
	Comment   string          `json:"comment,omitempty"`
	Disable   json.RawMessage `json:"disable,omitempty"`
	Guest     int64           `json:"guest,omitempty"`
	GuestName string          `json:"guest_name,omitempty"`
	JobNum    int64           `json:"jobnum,omitempty"`
	Schedule  string          `json:"schedule,omitempty"`
	Rate      *float64        `json:"rate,omitempty"`
	Removal   json.RawMessage `json:"removal,omitempty"`
	LastSync  int64           `json:"last_sync,omitempty"`
	LastTry   int64           `json:"last_try,omitempty"`
	NextSync  int64           `json:"next_sync,omitempty"`
	FailCount int64           `json:"fail_count,omitempty"`
	Duration  int64           `json:"duration,omitempty"`
	Status    string          `json:"status,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of disable and removal that
// PVE emits on some versions.
func (s *ReplicationJobStatus) UnmarshalJSON(data []byte) error {
	var raw replicationJobStatusRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.ID = raw.ID
	s.Type = raw.Type
	s.Target = raw.Target
	s.Comment = raw.Comment
	s.Disable = decodeBoolish(raw.Disable)
	s.Guest = raw.Guest
	s.GuestName = raw.GuestName
	s.JobNum = raw.JobNum
	s.Schedule = raw.Schedule
	s.Rate = raw.Rate
	s.Removal = decodeBoolish(raw.Removal)
	s.LastSync = raw.LastSync
	s.LastTry = raw.LastTry
	s.NextSync = raw.NextSync
	s.FailCount = raw.FailCount
	s.Duration = raw.Duration
	s.Status = raw.Status
	s.Error = raw.Error
	return nil
}

// ListNodeReplications enumerates GET /nodes/{node}/replication. A guest
// ID above zero filters server-side to that guest's jobs.
func (c *Client) ListNodeReplications(ctx context.Context, node string, guest int64) ([]ReplicationJobStatus, error) {
	path := "/nodes/" + url.PathEscape(node) + "/replication"
	if guest > 0 {
		path += "?guest=" + strconv.FormatInt(guest, 10)
	}
	var out []ReplicationJobStatus
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// replicationLogLine is one numbered line of a replication job log.
type replicationLogLine struct {
	N int    `json:"n"`
	T string `json:"t"`
}

// GetReplicationLog reads GET /nodes/{node}/replication/{id}/log and
// returns the log line texts in order.
func (c *Client) GetReplicationLog(ctx context.Context, node, id string) ([]string, error) {
	path := "/nodes/" + url.PathEscape(node) + "/replication/" + url.PathEscape(id) + "/log"
	var lines []replicationLogLine
	if err := c.Do(ctx, "GET", path, nil, &lines); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.T)
	}
	return out, nil
}

// ScheduleReplicationNow POSTs /nodes/{node}/replication/{id}/schedule_now,
// which queues the job to run as soon as possible, and returns the
// replication run's task UPID for the caller to wait on.
func (c *Client) ScheduleReplicationNow(ctx context.Context, node, id string) (string, error) {
	path := "/nodes/" + url.PathEscape(node) + "/replication/" + url.PathEscape(id) + "/schedule_now"
	var upid string
	if err := c.Do(ctx, "POST", path, nil, &upid); err != nil {
		return "", fmt.Errorf("scheduling replication job %s on %s: %w", id, node, err)
	}
	return upid, nil
}
