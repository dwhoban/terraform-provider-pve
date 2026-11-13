// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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
