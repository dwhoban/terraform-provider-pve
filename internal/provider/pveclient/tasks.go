// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskStatus mirrors the PVE /tasks/{upid}/status enum.
type TaskStatus string

const (
	TaskRunning TaskStatus = "running"
	TaskStopped TaskStatus = "stopped"
)

// TaskInfo is the decoded body of GET /nodes/{node}/tasks/{upid}/status. PVE
// keeps returning this body every poll until status flips to "stopped", at
// which point ExitStatus is "OK" on success or a worker-specific string on
// failure.
type TaskInfo struct {
	Status     TaskStatus `json:"status"`
	ExitStatus string     `json:"exitstatus"`
	PID        int64      `json:"pid,omitempty"`
}

// TaskLog is the decoded body of GET /nodes/{node}/tasks/{upid}/log: one
// array entry per log line (each line already carries a newline so callers
// usually join with "").
type TaskLog []string

// WaitForTaskOptions tunes a single WaitForTask call. Zero values fall back
// to the defaults described in the field docs.
type WaitForTaskOptions struct {
	// Interval between status polls. Default 500ms; clamped to a minimum
	// of 100ms.
	Interval time.Duration
	// Timeout for the whole wait. Default 30m. Honored via
	// context.WithTimeout wrapped around the polling loop. A zero value
	// disables the timeout and the caller's context is the only deadline.
	Timeout time.Duration
}

const (
	DefaultWaitInterval = 500 * time.Millisecond
	DefaultWaitTimeout  = 30 * time.Minute
	minWaitInterval     = 100 * time.Millisecond
	maxLogTailBytes     = 8 << 10 // 8 KiB captured on failure for diagnostics
)

// ErrTaskFailed is returned (via errors.Is) by WaitForTask when the task
// exited with a non-OK status. The wrapped *TaskError carries the upid,
var ErrTaskFailed = errors.New("pve task failed")

// TaskError is the concrete error type returned by WaitForTask on a
// non-OK exit. It satisfies errors.Is(err, ErrTaskFailed).
type TaskError struct {
	Node       string
	UPID       string
	ExitStatus string
	LogTail    string
}

func (e *TaskError) Error() string {
	tail := e.LogTail
	if tail == "" {
		tail = "<no log captured>"
	}
	return fmt.Sprintf("pve task %s on node %s exited with status %q: %s",
		e.UPID, e.Node, e.ExitStatus, strings.TrimRight(tail, "\n"))
}

func (e *TaskError) Unwrap() error { return ErrTaskFailed }

// WaitForTask polls GET /nodes/{node}/tasks/{upid}/status until the status
// flips to "stopped", then returns (*TaskInfo, nil) on success or a
// *TaskError wrapping ErrTaskFailed on a non-OK exit. The last 8 KiB of the
// task log are attached to *TaskError so callers can surface them as
// diagnostics.
//
// Polling honors both opts.Timeout (via context.WithTimeout) and the
// caller's context: whichever fires first stops the loop. On success the
// returned *TaskInfo has Status == TaskStopped and ExitStatus == "OK".
func (c *Client) WaitForTask(ctx context.Context, node, upid string, opts WaitForTaskOptions) (*TaskInfo, error) {
	if upid == "" {
		return nil, errors.New("pveclient: WaitForTask: upid is required")
	}
	if node == "" {
		return nil, errors.New("pveclient: WaitForTask: node is required")
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultWaitInterval
	}
	if interval < minWaitInterval {
		interval = minWaitInterval
	}
	waitCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Poll immediately on entry so we don't add interval latency for
	// already-completed tasks.
	for {
		info, err := c.getTaskStatus(waitCtx, node, upid)
		if err != nil {
			return nil, err
		}
		if info.Status == TaskStopped {
			if info.ExitStatus != "OK" {
				logTail, _ := c.fetchTaskLogTail(waitCtx, node, upid)
				return nil, &TaskError{
					Node:       node,
					UPID:       upid,
					ExitStatus: info.ExitStatus,
					LogTail:    logTail,
				}
			}
			return info, nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.Canceled) || errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("pveclient: WaitForTask(%s on %s): %w", upid, node, waitCtx.Err())
			}
			return nil, waitCtx.Err()
		case <-ticker.C:
		}
	}
}

// getTaskStatus performs a single GET against /nodes/{node}/tasks/{upid}/status.
func (c *Client) getTaskStatus(ctx context.Context, node, upid string) (*TaskInfo, error) {
	var info TaskInfo
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, upid)
	if err := c.Do(ctx, "GET", path, nil, &info); err != nil {
		return nil, fmt.Errorf("pveclient: get task status %s: %w", path, err)
	}
	return &info, nil
}

// fetchTaskLogTail reads the task log and returns the trailing maxLogTailBytes
// of text. A failure to fetch the log is non-fatal: the returned tail will
// be empty and the caller still gets a useful error.
func (c *Client) fetchTaskLogTail(ctx context.Context, node, upid string) (string, error) {
	var log TaskLog
	path := fmt.Sprintf("/nodes/%s/tasks/%s/log", node, upid)
	if err := c.Do(ctx, "GET", path, nil, &log); err != nil {
		return "", err
	}
	full := strings.Join(log, "")
	if len(full) <= maxLogTailBytes {
		return full, nil
	}
	return full[len(full)-maxLogTailBytes:], nil
}
