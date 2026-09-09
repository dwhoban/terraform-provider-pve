// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// p3BulkWantBody describes the decoded-JSON request body a bulk test
// expects: scalar and array values keyed by wire name.
type p3BulkWantBody map[string]any

// p3BulkPtr returns a pointer to v; shared by the bulk action tests.
func p3BulkPtr[T any](v T) *T { return &v }

// TestClient_ListNodeAppliances_Decodes covers GET /nodes/{node}/aplinfo.
// The pin declares the row objects as free-form, so the client's typed
// decode is the contract under test.
func TestClient_ListNodeAppliances_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/aplinfo" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{`+
				`"template":"ubuntu-24.04-standard_24.04-2_amd64.tar.zst",`+
				`"source":"www.turnkeylinux.org",`+
				`"infrastructure":"pve",`+
				`"description":"Ubuntu 24.04 JeOS",`+
				`"package":"ubuntu-24.04-standard",`+
				`"section":"system",`+
				`"version":"24.04-2",`+
				`"os":"ubuntu-24.04",`+
				`"info":"https://pve.proxmox.com/wiki/",`+
				`"manageurl":"http://<ip>:8006/",`+
				`"sha512sum":"abc123",`+
				`"architecture":"amd64"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	appliances, err := c.ListNodeAppliances(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeAppliances: %v", err)
	}
	if len(appliances) != 1 {
		t.Fatalf("want 1 appliance, got %d", len(appliances))
	}
	a := appliances[0]
	if a.Template != "ubuntu-24.04-standard_24.04-2_amd64.tar.zst" || a.Source != "www.turnkeylinux.org" {
		t.Fatalf("unexpected identity fields: %+v", a)
	}
	if a.Infrastructure != "pve" || a.Description != "Ubuntu 24.04 JeOS" || a.Package != "ubuntu-24.04-standard" {
		t.Fatalf("unexpected descriptive fields: %+v", a)
	}
	if a.Section != "system" || a.Version != "24.04-2" || a.OS != "ubuntu-24.04" {
		t.Fatalf("unexpected classification fields: %+v", a)
	}
	if a.Info != "https://pve.proxmox.com/wiki/" || a.ManageURL != "http://<ip>:8006/" || a.SHA512Sum != "abc123" || a.Architecture != "amd64" {
		t.Fatalf("unexpected remaining fields: %+v", a)
	}
}

// TestClient_GetNodeSubscription_Decodes covers GET /nodes/{node}/subscription
// with a fully populated row and asserts pointer presence for optional fields.
func TestClient_GetNodeSubscription_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/subscription" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{`+
				`"status":"active",`+
				`"key":"pve2c-1234567890",`+
				`"sockets":2,`+
				`"level":"c",`+
				`"message":"You have an active subscription",`+
				`"nextduedate":"2027-01-01",`+
				`"productname":"Proxmox VE Subscription",`+
				`"regdate":"2026-01-01",`+
				`"serverid":"0123456789abcdef",`+
				`"signature":"offline-signature",`+
				`"checktime":1750000000,`+
				`"url":"https://shop.proxmox.com"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	sub, err := c.GetNodeSubscription(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeSubscription: %v", err)
	}
	if sub.Status != "active" || sub.Level == nil || *sub.Level != "c" || sub.Sockets == nil || *sub.Sockets != 2 {
		t.Fatalf("unexpected subscription core: %+v", sub)
	}
	if sub.Key == nil || *sub.Key != "pve2c-1234567890" || sub.ServerID == nil || *sub.ServerID != "0123456789abcdef" {
		t.Fatalf("unexpected identity fields: %+v", sub)
	}
	if sub.NextDueDate == nil || *sub.NextDueDate != "2027-01-01" || sub.ProductName == nil || *sub.ProductName != "Proxmox VE Subscription" {
		t.Fatalf("unexpected descriptive fields: %+v", sub)
	}
	if sub.Message == nil || sub.RegDate == nil || sub.Signature == nil || sub.CheckTime == nil || sub.URL == nil {
		t.Fatalf("expected all optional fields populated: %+v", sub)
	}
}

// TestClient_GetNodeSubscription_Minimal asserts that a subscription-less
// node (status only) decodes with nil optional fields instead of failing.
func TestClient_GetNodeSubscription_Minimal(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"status":"notfound"}}`)
	})

	sub, err := c.GetNodeSubscription(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeSubscription: %v", err)
	}
	if sub.Status != "notfound" {
		t.Fatalf("status = %q, want notfound", sub.Status)
	}
	if sub.Key != nil || sub.Level != nil || sub.Sockets != nil || sub.NextDueDate != nil {
		t.Fatalf("expected nil optional fields, got: %+v", sub)
	}
}

// TestClient_NodeExecute_CountOnly asserts the POST body shape and that the
// per-command result objects are discarded: only the count is returned.
func TestClient_NodeExecute_CountOnly(t *testing.T) {
	var gotBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/execute" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"status":200,"data":null},{"status":200,"data":{"sensitive":"output that must not surface"}}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	commands := `[{"method":"GET","path":"version","args":{}}]`
	count, err := c.NodeExecute(context.Background(), "pve1", commands)
	if err != nil {
		t.Fatalf("NodeExecute: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if gotBody["commands"] != commands {
		t.Fatalf("commands body = %v, want %q", gotBody["commands"], commands)
	}
}

// TestClient_NodeAllGuests_BodyShapes walks the four per-node bulk verbs
// (startall, stopall, suspendall, migrateall) and asserts path, method,
// body shape, and the returned UPID.
func TestClient_NodeAllGuests_BodyShapes(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:6700:vstartall:root@pam:"
	cases := []struct {
		name string
		path string
		call func(c *Client) (string, error)
		want p3BulkWantBody
	}{
		{
			name: "startall",
			path: "/nodes/pve1/startall",
			call: func(c *Client) (string, error) {
				return c.StartAllNodeGuests(context.Background(), "pve1", StartAllNodeGuestsOptions{
					Force: p3BulkPtr(true), MaxWorkers: p3BulkPtr(int64(4)), Vms: "100,101",
				})
			},
			want: map[string]any{"force": true, "max-workers": float64(4), "vms": "100,101"},
		},
		{
			name: "stopall",
			path: "/nodes/pve1/stopall",
			call: func(c *Client) (string, error) {
				return c.StopAllNodeGuests(context.Background(), "pve1", StopAllNodeGuestsOptions{
					ForceStop: p3BulkPtr(true), MaxWorkers: p3BulkPtr(int64(2)), Timeout: p3BulkPtr(int64(300)), Vms: "100",
				})
			},
			want: map[string]any{"force-stop": true, "max-workers": float64(2), "timeout": float64(300), "vms": "100"},
		},
		{
			name: "suspendall",
			path: "/nodes/pve1/suspendall",
			call: func(c *Client) (string, error) {
				return c.SuspendAllNodeGuests(context.Background(), "pve1", SuspendAllNodeGuestsOptions{
					MaxWorkers: p3BulkPtr(int64(3)), Vms: "100",
				})
			},
			want: map[string]any{"max-workers": float64(3), "vms": "100"},
		},
		{
			name: "migrateall",
			path: "/nodes/pve1/migrateall",
			call: func(c *Client) (string, error) {
				return c.MigrateAllNodeGuests(context.Background(), "pve1", MigrateAllNodeGuestsOptions{
					Target: "pve2", MaxWorkers: p3BulkPtr(int64(1)), Vms: "100", WithLocalDisks: p3BulkPtr(true),
				})
			},
			want: map[string]any{"target": "pve2", "max-workers": float64(1), "vms": "100", "with-local-disks": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			var gotMethod string
			c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tc.path {
					gotMethod = r.Method
					body, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(body, &gotBody)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})

			gotUpid, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if gotMethod != http.MethodPost {
				t.Fatalf("method = %s, want POST", gotMethod)
			}
			if gotUpid != upid {
				t.Fatalf("upid = %q, want %q", gotUpid, upid)
			}
			if len(gotBody) != len(tc.want) {
				t.Fatalf("body keys = %v, want exactly %v", gotBody, tc.want)
			}
			for k, v := range tc.want {
				if gotBody[k] != v {
					t.Fatalf("body[%q] = %v, want %v", k, gotBody[k], v)
				}
			}
		})
	}
}

// TestClient_NodeAllGuests_EmptyOptions asserts that unset optional fields
// are omitted from the request body entirely.
func TestClient_NodeAllGuests_EmptyOptions(t *testing.T) {
	var gotBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nodes/pve1/startall" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:1:1:1:"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := c.StartAllNodeGuests(context.Background(), "pve1", StartAllNodeGuestsOptions{}); err != nil {
		t.Fatalf("StartAllNodeGuests: %v", err)
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected empty body, got %v", gotBody)
	}
}

// TestClient_BulkGuests_BodyShapes walks the four cluster bulk verbs
// (/cluster/bulk-action/guest/{start,shutdown,suspend,migrate}) and asserts
// path, method, body shape, and the returned UPID.
func TestClient_BulkGuests_BodyShapes(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:6700:bulkstart:root@pam:"
	cases := []struct {
		name string
		path string
		call func(c *Client) (string, error)
		want p3BulkWantBody
	}{
		{
			name: "start",
			path: "/cluster/bulk-action/guest/start",
			call: func(c *Client) (string, error) {
				return c.BulkStartGuests(context.Background(), BulkStartGuestsOptions{
					MaxWorkers: p3BulkPtr(int64(8)), Timeout: p3BulkPtr(int64(60)), Vms: []int64{100, 101},
				})
			},
			want: map[string]any{"max-workers": float64(8), "timeout": float64(60), "vms": []any{float64(100), float64(101)}},
		},
		{
			name: "shutdown",
			path: "/cluster/bulk-action/guest/shutdown",
			call: func(c *Client) (string, error) {
				return c.BulkShutdownGuests(context.Background(), BulkShutdownGuestsOptions{
					ForceStop: p3BulkPtr(false), MaxWorkers: p3BulkPtr(int64(2)), Timeout: p3BulkPtr(int64(180)), Vms: []int64{100},
				})
			},
			want: map[string]any{"force-stop": false, "max-workers": float64(2), "timeout": float64(180), "vms": []any{float64(100)}},
		},
		{
			name: "suspend",
			path: "/cluster/bulk-action/guest/suspend",
			call: func(c *Client) (string, error) {
				return c.BulkSuspendGuests(context.Background(), BulkSuspendGuestsOptions{
					MaxWorkers: p3BulkPtr(int64(4)), StateStorage: "local-zfs", ToDisk: p3BulkPtr(true), Vms: []int64{100},
				})
			},
			want: map[string]any{"max-workers": float64(4), "statestorage": "local-zfs", "to-disk": true, "vms": []any{float64(100)}},
		},
		{
			name: "migrate",
			path: "/cluster/bulk-action/guest/migrate",
			call: func(c *Client) (string, error) {
				return c.BulkMigrateGuests(context.Background(), BulkMigrateGuestsOptions{
					Target: "pve2", Online: p3BulkPtr(true), Vms: []int64{100}, WithLocalDisks: p3BulkPtr(true),
				})
			},
			want: map[string]any{"target": "pve2", "online": true, "vms": []any{float64(100)}, "with-local-disks": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			var gotMethod string
			c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tc.path {
					gotMethod = r.Method
					body, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(body, &gotBody)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})

			gotUpid, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if gotMethod != http.MethodPost {
				t.Fatalf("method = %s, want POST", gotMethod)
			}
			if gotUpid != upid {
				t.Fatalf("upid = %q, want %q", gotUpid, upid)
			}
			gotJSON, _ := json.Marshal(gotBody)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("body = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestClient_GetGuestAgentFacts_Merges covers the merged decode of the pin's
// GET agent commands: info, get-host-name, get-osinfo, get-time,
// get-timezone, get-vcpus, get-users, and network-get-interfaces.
func TestClient_GetGuestAgentFacts_Merges(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/nodes/pve1/qemu/100/agent/info":
			_, _ = io.WriteString(w, `{"data":{"result":{"version":"1.3.0"}}}`)
		case "/nodes/pve1/qemu/100/agent/get-host-name":
			_, _ = io.WriteString(w, `{"data":{"result":{"name":"web1"}}}`)
		case "/nodes/pve1/qemu/100/agent/get-osinfo":
			_, _ = io.WriteString(w, `{"data":{"result":{`+
				`"id":"ubuntu","id_like":"debian","machine":"x86_64",`+
				`"kernel_name":"Linux","kernel_release":"5.15.0-91-generic","kernel_version":"#101-Ubuntu",`+
				`"name":"Ubuntu","pretty_name":"Ubuntu 22.04.5 LTS",`+
				`"variant":"server","variant_id":"server",`+
				`"version":"22.04.5 LTS (Jammy Jellyfish)","version_id":"22.04",`+
				`"home_url":"https://www.ubuntu.com/"}}}`)
		case "/nodes/pve1/qemu/100/agent/get-time":
			_, _ = io.WriteString(w, `{"data":{"result":{"timestamp":1750000000123456789,"time-zone":"UTC"}}}`)
		case "/nodes/pve1/qemu/100/agent/get-timezone":
			_, _ = io.WriteString(w, `{"data":{"result":{"zone":"Europe/Berlin","offset":3600}}}`)
		case "/nodes/pve1/qemu/100/agent/get-vcpus":
			_, _ = io.WriteString(w, `{"data":{"result":[{"online":true,"can-offline":false,"logical-id":0,"vcpus-count":2}]}}`)
		case "/nodes/pve1/qemu/100/agent/get-users":
			_, _ = io.WriteString(w, `{"data":{"result":[{"username":"root","domain":"internal","login-time":1750000000}]}}`)
		case "/nodes/pve1/qemu/100/agent/network-get-interfaces":
			_, _ = io.WriteString(w, `{"data":{"result":[{"name":"eth0","hardware-address":"52:54:00:aa:bb:cc",`+
				`"ip-addresses":[{"ip-address":"192.168.1.10","ip-address-type":"ipv4","prefix":24}]}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	facts, err := c.GetGuestAgentFacts(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetGuestAgentFacts: %v", err)
	}
	if facts.Version != "1.3.0" || facts.HostName != "web1" {
		t.Fatalf("unexpected agent identity: %+v", facts)
	}
	if facts.OSName != "Ubuntu" || facts.OSPrettyName != "Ubuntu 22.04.5 LTS" || facts.OSID != "ubuntu" ||
		facts.OSIDLike != "debian" || facts.OSMachine != "x86_64" || facts.OSKernelName != "Linux" ||
		facts.OSKernelRelease != "5.15.0-91-generic" || facts.OSKernelVersion != "#101-Ubuntu" ||
		facts.OSVariant != "server" || facts.OSVariantID != "server" || facts.OSVersion != "22.04.5 LTS (Jammy Jellyfish)" ||
		facts.OSVersionID != "22.04" || facts.OSHomeURL != "https://www.ubuntu.com/" {
		t.Fatalf("unexpected osinfo merge: %+v", facts)
	}
	if facts.Time == nil || *facts.Time != 1750000000123456789 {
		t.Fatalf("unexpected get-time merge: %+v", facts.Time)
	}
	if facts.TimeZone != "Europe/Berlin" {
		t.Fatalf("timezone = %q, want Europe/Berlin", facts.TimeZone)
	}
	if facts.VCPUs == nil || *facts.VCPUs != 1 {
		t.Fatalf("vcpus = %v, want 1", facts.VCPUs)
	}
	if len(facts.Users) != 1 || facts.Users[0] != "root" {
		t.Fatalf("users = %v, want [root]", facts.Users)
	}
	if len(facts.Interfaces) != 1 || facts.Interfaces[0].Name != "eth0" ||
		facts.Interfaces[0].HardwareAddress != "52:54:00:aa:bb:cc" ||
		len(facts.Interfaces[0].IPAddresses) != 1 || facts.Interfaces[0].IPAddresses[0].Address != "192.168.1.10" ||
		facts.Interfaces[0].IPAddresses[0].Type != "ipv4" || facts.Interfaces[0].IPAddresses[0].Prefix != 24 {
		t.Fatalf("unexpected interfaces merge: %+v", facts.Interfaces)
	}
}

// TestClient_GetGuestAgentFacts_AgentUnavailable asserts that a failing
// info command (guest agent not running) surfaces as an error.
func TestClient_GetGuestAgentFacts_AgentUnavailable(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"data":null,"errors":"QEMU guest agent is not running\n500 Can't connect to QEMU guest agent\n"}`)
	})

	if _, err := c.GetGuestAgentFacts(context.Background(), "pve1", 100); err == nil {
		t.Fatal("expected error when guest agent is unavailable")
	}
}

// TestClient_GetGuestAgentFacts_ToleratesUnsupported asserts that commands
// unsupported by the installed guest agent degrade to nil facts instead of
// failing the whole read.
func TestClient_GetGuestAgentFacts_ToleratesUnsupported(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/nodes/pve1/qemu/100/agent/info":
			_, _ = io.WriteString(w, `{"data":{"result":{"version":"1.1.0"}}}`)
		case "/nodes/pve1/qemu/100/agent/get-host-name":
			_, _ = io.WriteString(w, `{"data":{"result":{"name":"web1"}}}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"data":null,"errors":"the guest agent does not support this command"}`)
		}
	})

	facts, err := c.GetGuestAgentFacts(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetGuestAgentFacts: %v", err)
	}
	if facts.Version != "1.1.0" || facts.HostName != "web1" {
		t.Fatalf("unexpected supported facts: %+v", facts)
	}
	if facts.OSName != "" || facts.Time != nil || facts.TimeZone != "" || facts.VCPUs != nil || facts.Users != nil || facts.Interfaces != nil {
		t.Fatalf("expected nil facts for unsupported commands: %+v", facts)
	}
}
