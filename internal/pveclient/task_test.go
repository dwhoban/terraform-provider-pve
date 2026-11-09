// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseUPID(t *testing.T) {
	raw := "UPID:node1:0000ABCD:00000123:6789DEF0:vzstart:100:root@pam:"
	u, err := ParseUPID(raw)
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Node != "node1" || u.Type != "vzstart" || u.ID != "100" || u.Extra != "root@pam:" {
		t.Errorf("parsed %+v", u)
	}
	if u.PID != 0xABCD || u.Pstart != 0x123 || u.StartTime != 0x6789DEF0 {
		t.Errorf("hex fields wrong: %+v", u)
	}
	if u.String() != raw {
		t.Errorf("String() = %q, want %q", u.String(), raw)
	}
}

func TestParseUPIDInvalid(t *testing.T) {
	if _, err := ParseUPID("not-a-upid"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseUPID("UPID:node:ZZZ:1:1:vzstart:100:"); err == nil {
		t.Fatal("expected error for non-hex pid")
	}
}

func taskTestClient(t *testing.T, statusHandler, logHandler http.HandlerFunc) *Client {
	t.Helper()
	c, _ := testClient(t, Credentials{APIToken: "root@pam!t=abc"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			statusHandler(w, r)
		case strings.HasSuffix(r.URL.Path, "/log"):
			logHandler(w, r)
		default:
			writeJSON(w, http.StatusNotFound, `{"data":null}`)
		}
	}))
	return c
}

func TestWaitForTaskOK(t *testing.T) {
	var polls atomic.Int32
	c := taskTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			if polls.Add(1) == 1 {
				writeJSON(w, http.StatusOK, `{"data":{"status":"running"}}`)
				return
			}
			writeJSON(w, http.StatusOK, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("task log should not be read on success")
		})

	upid := "UPID:node1:0000ABCD:00000123:6789DEF0:vzstart:100:root@pam:"
	if err := c.WaitForTask(context.Background(), upid, TaskPollOptions{Interval: time.Millisecond}); err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
	if polls.Load() < 2 {
		t.Errorf("polls = %d, want >= 2", polls.Load())
	}
}

func TestWaitForTaskFailureIncludesLogTail(t *testing.T) {
	c := taskTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, `{"data":{"status":"stopped","exitstatus":"command error"}}`)
		},
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, `{"data":[{"n":1,"t":"line1"},{"n":2,"t":"boom"}]}`)
		})

	err := c.WaitForTask(context.Background(), "UPID:node1:1:1:1:vzstart:100:", TaskPollOptions{Interval: time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"command error", "boom", "vzstart"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q: %v", want, err)
		}
	}
}

func TestWaitForTaskTimeout(t *testing.T) {
	c := taskTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, `{"data":{"status":"running"}}`)
		},
		func(w http.ResponseWriter, r *http.Request) {},
	)

	err := c.WaitForTask(context.Background(), "UPID:node1:1:1:1:vzstart:100:", TaskPollOptions{Interval: time.Millisecond, Timeout: 30 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want timeout error, got %v", err)
	}
}
