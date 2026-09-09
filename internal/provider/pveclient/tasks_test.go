// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeUpid is a realistic-looking PVE task identifier used across the
// WaitForTask tests; it does not need to be valid because the fake server
// only pattern-matches on the literal string.
const fakeUpid = "UPID:pve1:00001234:12345678:ZFS:operator:root@pam:"

// newFakePVETokenServer spins up an httptest server and returns a Client
// configured against it. Tests register a handler per path and assert
// traffic on it. The token in the auth header is the same one the test
// injects; the server does not validate it.
func newFakePVETokenServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(Credentials{
		Token:    "root@pam!tf=test",
		Endpoint: srv.URL,
		Insecure: "true",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestClient_WaitForTask_Success walks a task from running to stopped with
// exitstatus OK and asserts the returned TaskInfo and the number of polls.
func TestClient_WaitForTask_Success(t *testing.T) {
	var polls int32
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/nodes/pve1/tasks/"+fakeUpid+"/status" {
			n := atomic.AddInt32(&polls, 1)
			if n < 3 {
				_, _ = io.WriteString(w, `{"data":{"status":"running","exitstatus":"","pid":42}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK","pid":42}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	info, err := c.WaitForTask(context.Background(), "pve1", fakeUpid, WaitForTaskOptions{Interval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
	if info == nil || info.Status != TaskStopped || info.ExitStatus != "OK" {
		t.Fatalf("TaskInfo = %+v, want stopped/OK", info)
	}
	if got := atomic.LoadInt32(&polls); got < 3 {
		t.Fatalf("polls = %d, want >= 3", got)
	}
}

// TestClient_WaitForTask_AlreadyStopped covers the fast path where the
// first poll already reports a stopped status.
func TestClient_WaitForTask_AlreadyStopped(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK","pid":42}}`)
	})
	info, err := c.WaitForTask(context.Background(), "pve1", fakeUpid, WaitForTaskOptions{Interval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
	if info.ExitStatus != "OK" {
		t.Fatalf("ExitStatus = %q, want OK", info.ExitStatus)
	}
}

// TestClient_WaitForTask_FailureCapturesLogTail verifies that a non-OK
// exit produces a *TaskError wrapping ErrTaskFailed and that the tail of
// the task log is attached.
func TestClient_WaitForTask_FailureCapturesLogTail(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"command 'foo' failed: exit code 2","pid":7}}`)
		case strings.HasSuffix(r.URL.Path, "/log"):
			_, _ = io.WriteString(w, `{"data":["TASK ERROR: command failed\n","see journal for details\n"]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := c.WaitForTask(context.Background(), "pve1", fakeUpid, WaitForTaskOptions{Interval: 10 * time.Millisecond})
	if err == nil {
		t.Fatal("expected error from non-OK task, got nil")
	}
	if !errors.Is(err, ErrTaskFailed) {
		t.Fatalf("err does not wrap ErrTaskFailed: %v", err)
	}
	var taskErr *TaskError
	if !errors.As(err, &taskErr) {
		t.Fatalf("err is not a *TaskError: %T", err)
	}
	if taskErr.ExitStatus != "command 'foo' failed: exit code 2" {
		t.Fatalf("ExitStatus = %q", taskErr.ExitStatus)
	}
	if !strings.Contains(taskErr.LogTail, "TASK ERROR") {
		t.Fatalf("LogTail missing log content: %q", taskErr.LogTail)
	}
	if !strings.Contains(err.Error(), "exit code 2") {
		t.Fatalf("Error() missing exit status: %s", err.Error())
	}
}

// TestClient_WaitForTask_Timeout confirms that exceeding WaitForTaskOptions.Timeout
// surfaces a deadline-exceeded error rather than hanging.
func TestClient_WaitForTask_Timeout(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"status":"running","exitstatus":"","pid":1}}`)
	})

	_, err := c.WaitForTask(context.Background(), "pve1", "UPID:still-running",
		WaitForTaskOptions{Interval: 5 * time.Millisecond, Timeout: 30 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err is not deadline-exceeded: %v", err)
	}
}

// TestClient_WaitForTask_ContextCancel asserts a caller-driven cancellation
// short-circuits the poll loop.
func TestClient_WaitForTask_ContextCancel(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"status":"running","exitstatus":"","pid":1}}`)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := c.WaitForTask(ctx, "pve1", "UPID:never-stops", WaitForTaskOptions{Interval: 5 * time.Millisecond})
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err is not canceled: %v", err)
	}
}

// TestClient_WaitForTask_RequiresArguments guards the empty-argument paths.
func TestClient_WaitForTask_RequiresArguments(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s", r.URL.Path)
	})
	if _, err := c.WaitForTask(context.Background(), "pve1", "", WaitForTaskOptions{}); err == nil {
		t.Error("expected error for empty upid")
	}
	if _, err := c.WaitForTask(context.Background(), "", "UPID", WaitForTaskOptions{}); err == nil {
		t.Error("expected error for empty node")
	}
}
