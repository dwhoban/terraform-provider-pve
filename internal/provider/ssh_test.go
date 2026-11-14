// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func cephProbeClient(t *testing.T, status int, body string) *pveclient.Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL: srv.URL, Insecure: true, RetryMax: 0,
		Credentials: pveclient.Credentials{APIToken: "root@pam!t=s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCephInstalledProbe(t *testing.T) {
	ctx := t.Context()
	cases := []struct {
		name    string
		status  int
		body    string
		want    bool
		wantErr bool
	}{
		{"binaries present", 200, `{"data":[]}`, true, false},
		{"not installed", 500, `{"message":"binary not installed: /usr/bin/ceph-mon"}`, false, false},
		{"installed but uninitialized", 500, `{"message":"no such cluster configuration"}`, true, false},
		{"forbidden says nothing about packages", 403, `{"message":"Permission check failed"}`, false, true},
	}
	for _, tc := range cases {
		got, err := cephInstalled(ctx, cephProbeClient(t, tc.status, tc.body), "pve01")
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: got (%v, %v), want (%v, err=%v)", tc.name, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestCephInstallArgv(t *testing.T) {
	got := cephInstallArgv(cephInstallModel{Repository: types.StringNull(), Version: types.StringNull()})
	if want := []string{"/usr/sbin/pveceph", "install", "--repository", "no-subscription"}; !reflect.DeepEqual(got, want) {
		t.Errorf("defaults: %v", got)
	}
	got = cephInstallArgv(cephInstallModel{
		Repository: types.StringValue("test"), Version: types.StringValue("squid"), AllowExperimental: types.BoolValue(true),
	})
	want := []string{"/usr/sbin/pveceph", "install", "--repository", "test", "--version", "squid", "--allow-experimental"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("full: %v", got)
	}
}

func nullSSH() *sshModel {
	return &sshModel{
		Username: types.StringNull(), Password: types.StringNull(), PrivateKey: types.StringNull(),
		Agent: types.BoolNull(), AgentSocket: types.StringNull(), KnownHostsFile: types.StringNull(),
		InsecureIgnoreHostKey: types.BoolNull(), RootTicket: types.BoolNull(),
	}
}

func TestSSHConfiguredOnlyWhenRequested(t *testing.T) {
	none := func(string) string { return "" }
	if sshConfigured(nil, none) || sshConfigured(nullSSH(), none) {
		t.Fatal("an absent ssh block must not enable SSH")
	}
	m := nullSSH()
	m.Agent = types.BoolValue(true)
	if !sshConfigured(m, none) {
		t.Fatal("ssh.agent enables SSH")
	}
	if !sshConfigured(nil, func(k string) string {
		if k == envSSHPassword {
			return "x"
		}
		return ""
	}) {
		t.Fatal("PVE_SSH_PASSWORD enables SSH")
	}
}

func TestSSHUserAndPasswordDefaults(t *testing.T) {
	none := func(string) string { return "" }
	var d diag.Diagnostics

	// PAM API token: user inferred, but no password to inherit and no
	// other method → configuration error, not a silent no-auth runner.
	if r := buildSSHRunner(nullSSH(), none, pveclient.Credentials{APIToken: "root@pam!tf=secret"}, &d); r != nil || !d.HasError() {
		t.Fatal("token auth without an ssh credential must be rejected")
	}

	// Username/password API login of a PAM user: password inherited.
	d = diag.Diagnostics{}
	if r := buildSSHRunner(nullSSH(), none, pveclient.Credentials{Username: "root@pam", Password: "pw"}, &d); r == nil || d.HasError() {
		t.Fatalf("pam login should inherit user and password: %v", d)
	}

	// Non-PAM API token: the SSH user cannot be inferred.
	d = diag.Diagnostics{}
	m := nullSSH()
	m.Agent = types.BoolValue(true)
	if r := buildSSHRunner(m, none, pveclient.Credentials{APIToken: "svc@pve!tf=secret"}, &d); r != nil || !d.HasError() {
		t.Fatal("non-PAM token needs an explicit ssh.username")
	}

	// root_ticket demands a root shell.
	d = diag.Diagnostics{}
	m = nullSSH()
	m.Username, m.Agent, m.RootTicket = types.StringValue("terraform"), types.BoolValue(true), types.BoolValue(true)
	if r := buildSSHRunner(m, none, pveclient.Credentials{APIToken: "root@pam!tf=s"}, &d); r != nil || !d.HasError() {
		t.Fatal("root_ticket with a non-root ssh user must be rejected")
	}
}

func TestQDeviceSetupArgv(t *testing.T) {
	cases := []struct {
		network string
		force   bool
		want    []string
	}{
		{"", false, []string{"/usr/bin/pvecm", "qdevice", "setup", "10.0.0.50"}},
		{"", true, []string{"/usr/bin/pvecm", "qdevice", "setup", "10.0.0.50", "--force"}},
		{"10.0.0.0/24", false, []string{"/usr/bin/pvecm", "qdevice", "setup", "10.0.0.50", "--network", "10.0.0.0/24"}},
		{"10.0.0.0/24", true, []string{"/usr/bin/pvecm", "qdevice", "setup", "10.0.0.50", "--force", "--network", "10.0.0.0/24"}},
	}
	for _, tc := range cases {
		if got := qdeviceSetupArgv("10.0.0.50", tc.network, tc.force); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("network=%q force=%v: %v, want %v", tc.network, tc.force, got, tc.want)
		}
	}
	if got, want := qdeviceRemoveArgv(), []string{"/usr/bin/pvecm", "qdevice", "remove"}; !reflect.DeepEqual(got, want) {
		t.Errorf("remove: %v", got)
	}
}

func TestParseQDeviceStatus(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		wantOK   bool
		wantHost string
		wantErr  bool
	}{
		{"empty object", `{}`, false, "", false},
		{"null", `null`, false, "", false},
		{"no payload", ``, false, "", false},
		{"ipv4", `{"QNetd host":"10.0.0.50:5403","State":"Connected"}`, true, "10.0.0.50", false},
		{"ipv6", `{"QNetd host":"[fd00::5]:5403","State":"Connected"}`, true, "fd00::5", false},
		{"no port", `{"QNetd host":"10.0.0.50"}`, true, "10.0.0.50", false},
		{"other keys only", `{"State":"Connected"}`, false, "", false},
		{"not an object", `[1]`, false, "", true},
	}
	for _, tc := range cases {
		st, ok, err := parseQDeviceStatus(json.RawMessage(tc.payload))
		if (err != nil) != tc.wantErr || ok != tc.wantOK || st.Host != tc.wantHost {
			t.Errorf("%s: got (%+v, %v, %v), want host=%q ok=%v err=%v", tc.name, st, ok, err, tc.wantHost, tc.wantOK, tc.wantErr)
		}
	}
	st, _, _ := parseQDeviceStatus(json.RawMessage(`{"Algorithm":"Fifty-Fifty split","Echo reply":"Supported","Model":"Net","QNetd host":"10.0.0.50:5403","State":"Connected","Tie-breaker":"Node with lowest ID"}`))
	want := qdeviceStatus{QNetdHost: "10.0.0.50:5403", Host: "10.0.0.50", State: "Connected", Algorithm: "Fifty-Fifty split", Model: "Net", TieBreak: "Node with lowest ID"}
	if st != want {
		t.Errorf("full status: %+v, want %+v", st, want)
	}
}

// qdeviceFakeShell records commands and flips the fake PVE's QDevice state.
type qdeviceFakeShell struct {
	configured *atomic.Bool
	runs       [][]string
}

func (s *qdeviceFakeShell) Run(_ context.Context, _ string, argv []string, _ string) (pveclient.ShellOutput, error) {
	s.runs = append(s.runs, argv)
	s.configured.Store(argv[2] == "setup")
	return pveclient.ShellOutput{}, nil
}

func qdeviceTestResource(t *testing.T, initiallyConfigured bool) (*clusterQDeviceResource, *qdeviceFakeShell) {
	t.Helper()
	var configured atomic.Bool
	configured.Store(initiallyConfigured)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"data":{}}`
		if configured.Load() {
			body = `{"data":{"QNetd host":"10.0.0.50:5403","State":"Connected","Model":"Net","Algorithm":"Fifty-Fifty split","Tie-breaker":"Node with lowest ID"}}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	shell := &qdeviceFakeShell{configured: &configured}
	c, err := pveclient.NewClient(pveclient.Config{
		URL: srv.URL, Insecure: true, RetryMax: 0, Shell: shell,
		Credentials: pveclient.Credentials{APIToken: "root@pam!t=s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &clusterQDeviceResource{client: c}, shell
}

func qdeviceTestModel(address string) clusterQDeviceModel {
	return clusterQDeviceModel{
		Node: types.StringValue("pve01"), Address: types.StringValue(address),
		Network: types.StringNull(), Force: types.BoolValue(true),
	}
}

func TestQDeviceProvisionAdoptsExisting(t *testing.T) {
	r, shell := qdeviceTestResource(t, true)
	m := qdeviceTestModel("10.0.0.50")
	if d := r.provision(t.Context(), &m); d.HasError() {
		t.Fatal(d)
	}
	if len(shell.runs) != 0 {
		t.Errorf("setup must not run for a matching QDevice: %v", shell.runs)
	}
	if m.ID.ValueString() != "10.0.0.50" || m.State.ValueString() != "Connected" {
		t.Errorf("model not filled: %+v", m)
	}

	other := qdeviceTestModel("10.0.0.51")
	if d := r.provision(t.Context(), &other); !d.HasError() {
		t.Error("a QDevice with another host must be rejected")
	}
	if len(shell.runs) != 0 {
		t.Errorf("setup must not run over a foreign QDevice: %v", shell.runs)
	}
}

func TestQDeviceProvisionRunsSetup(t *testing.T) {
	r, shell := qdeviceTestResource(t, false)
	m := qdeviceTestModel("10.0.0.50")
	if d := r.provision(t.Context(), &m); d.HasError() {
		t.Fatal(d)
	}
	want := [][]string{{"/usr/bin/pvecm", "qdevice", "setup", "10.0.0.50", "--force"}}
	if !reflect.DeepEqual(shell.runs, want) {
		t.Errorf("runs = %v, want %v", shell.runs, want)
	}
	if m.QNetdHost.ValueString() != "10.0.0.50:5403" || m.Model.ValueString() != "Net" {
		t.Errorf("model not filled: %+v", m)
	}

	bad := qdeviceTestModel("qnetd.example")
	if d := r.provision(t.Context(), &bad); !d.HasError() {
		t.Error("a hostname must be rejected")
	}
}

func TestQDeviceProvisionNeedsSSH(t *testing.T) {
	c, err := pveclient.NewClient(pveclient.Config{
		URL: "https://127.0.0.1:1", Insecure: true, RetryMax: 0,
		Credentials: pveclient.Credentials{APIToken: "root@pam!t=s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := qdeviceTestModel("10.0.0.50")
	if d := (&clusterQDeviceResource{client: c}).provision(t.Context(), &m); !d.HasError() {
		t.Fatal("provision without ssh must fail")
	}
}
