// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// UPID identifies an asynchronous Proxmox task, of the form
// UPID:node:pid:pstart:starttime:type:id:extra.
type UPID struct {
	Node      string
	PID       uint64
	Pstart    uint64
	StartTime uint64
	Type      string
	ID        string
	Extra     string

	raw string
}

var upidRe = regexp.MustCompile(`^UPID:([^:]+):([0-9A-Fa-f]+):([0-9A-Fa-f]+):([0-9A-Fa-f]+):([A-Za-z]+):([^:]*):(.*)$`)

// ParseUPID parses the raw UPID string returned by mutating PVE endpoints.
func ParseUPID(raw string) (UPID, error) {
	m := upidRe.FindStringSubmatch(raw)
	if m == nil {
		return UPID{}, fmt.Errorf("invalid UPID %q", raw)
	}
	pid, err := strconv.ParseUint(m[2], 16, 64)
	if err != nil {
		return UPID{}, fmt.Errorf("invalid UPID %q: pid: %w", raw, err)
	}
	pstart, err := strconv.ParseUint(m[3], 16, 64)
	if err != nil {
		return UPID{}, fmt.Errorf("invalid UPID %q: pstart: %w", raw, err)
	}
	startTime, err := strconv.ParseUint(m[4], 16, 64)
	if err != nil {
		return UPID{}, fmt.Errorf("invalid UPID %q: starttime: %w", raw, err)
	}
	return UPID{
		Node: m[1], PID: pid, Pstart: pstart, StartTime: startTime,
		Type: m[5], ID: m[6], Extra: m[7],
		raw: raw,
	}, nil
}

// String returns the original raw UPID.
func (u UPID) String() string { return u.raw }

// TaskPollOptions tunes WaitForTask.
type TaskPollOptions struct {
	// Interval between status polls. Defaults to 1s.
	Interval time.Duration
	// Timeout bounds the total wait. Defaults to 30 minutes (PVE tasks
	// such as full-disk clones, migrations, and backups routinely run
	// longer than the old 5-minute cap).
	Timeout time.Duration
	// LogTailLines is how many trailing task-log lines to include in a
	// failure error. Defaults to 10.
	LogTailLines int
	// OnWait, when set, is invoked after each poll with the elapsed time
	// and last status so callers can report progress.
	OnWait func(elapsed time.Duration, status string)
}

// TaskStatus is the response of GET /nodes/{node}/tasks/{upid}/status.
type TaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

// taskStatusResponse mirrors the API envelope.
type taskLogLine struct {
	N int    `json:"n"`
	T string `json:"t"`
}

// WaitForTask polls the task identified by rawUPID until it stops or the
// timeout expires. It returns nil when the task exited with status OK;
// otherwise the error carries the task's exit status and the tail of its log.
func (c *Client) WaitForTask(ctx context.Context, rawUPID string, opts TaskPollOptions) error {
	upid, err := ParseUPID(rawUPID)
	if err != nil {
		return err
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTaskTimeout
	}
	tailLines := opts.LogTailLines
	if tailLines <= 0 {
		tailLines = 10
	}

	deadline := time.Now().Add(timeout)
	start := time.Now()
	var last *TaskStatus
	for {
		st, err := c.getTaskStatus(ctx, upid)
		if err != nil {
			// A single transient poll failure (connection reset, one 5xx)
			// must not abort a healthy long-running task; keep polling
			// until the deadline or context cancellation.
			if ctx.Err() != nil || time.Now().After(deadline) {
				return err
			}
		} else {
			last = st
			if opts.OnWait != nil {
				status := st.Status
				if st == nil {
					status = ""
				}
				opts.OnWait(time.Since(start).Round(time.Second), status)
			}
		}
		if st != nil && st.Status == "stopped" {
			if st.ExitStatus == "OK" {
				return nil
			}
			tail, tailErr := c.getTaskLogTail(ctx, upid, tailLines)
			if tailErr != nil && tail == "" {
				return fmt.Errorf("task %s (%s %s) failed: %s", upid.raw, upid.Type, upid.ID, st.ExitStatus)
			}
			return fmt.Errorf("task %s (%s %s) failed: %s\ntask log tail:\n%s", upid.raw, upid.Type, upid.ID, st.ExitStatus, tail)
		}
		if time.Now().After(deadline) {
			lastStatus := "unknown"
			if last != nil {
				lastStatus = last.Status
			}
			return fmt.Errorf("timed out after %s waiting for task %s (%s %s): last status %q", timeout, upid.raw, upid.Type, upid.ID, lastStatus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (c *Client) getTaskStatus(ctx context.Context, upid UPID) (*TaskStatus, error) {
	out := &TaskStatus{}
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", upid.Node, upid.raw)
	if err := c.do(ctx, "GET", path, nil, out); err != nil {
		return nil, fmt.Errorf("reading task status: %w", err)
	}
	return out, nil
}

// getTaskLogTail returns the last n lines of the task log, newline-joined.
func (c *Client) getTaskLogTail(ctx context.Context, upid UPID, n int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/tasks/%s/log", upid.Node, upid.raw)
	var lines []taskLogLine
	if err := c.do(ctx, "GET", path, nil, &lines); err != nil {
		return "", fmt.Errorf("reading task log: %w", err)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := ""
	for _, l := range lines {
		out += l.T + "\n"
	}
	return out, nil
}
