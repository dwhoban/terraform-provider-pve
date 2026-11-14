// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pvessh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const testPassword = "pw"

// fakeNode is an in-process SSH server that records exec requests and
// answers them with a canned (stdout, exit status).
type fakeNode struct {
	addr    string
	hostKey ssh.PublicKey

	mu       sync.Mutex
	commands []string
	stdins   []string
	reply    func(cmd string) (string, uint32)
}

func startFakeNode(t *testing.T, user string) *fakeNode {
	const password = testPassword
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(pw) == password {
				return nil, nil
			}
			return nil, os.ErrPermission
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	n := &fakeNode{addr: ln.Addr().String(), hostKey: signer.PublicKey(), reply: func(string) (string, uint32) { return "", 0 }}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go n.serve(conn, cfg)
		}
	}()
	return n
}

func (n *fakeNode) serve(conn net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close() //nolint:errcheck
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				if r.Type != "exec" {
					_ = r.Reply(false, nil)
					continue
				}
				var p struct{ Cmd string }
				_ = ssh.Unmarshal(r.Payload, &p)
				_ = r.Reply(true, nil)
				buf := make([]byte, 256)
				m, _ := ch.Read(buf) // stdin (EOF immediately when none was sent)
				n.mu.Lock()
				n.commands = append(n.commands, p.Cmd)
				n.stdins = append(n.stdins, string(buf[:m]))
				out, status := n.reply(p.Cmd)
				n.mu.Unlock()
				if status == 0 {
					_, _ = ch.Write([]byte(out))
				} else {
					_, _ = ch.Stderr().Write([]byte(out))
				}
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
				_ = ch.Close()
			}
		}()
	}
}

func (n *fakeNode) config(user string) Config {
	host, port, _ := net.SplitHostPort(n.addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return Config{
		Username: user, Password: testPassword, InsecureIgnoreHostKey: true,
		Nodes: []Node{{Name: "pve01", Address: host, Port: p}},
	}
}

func TestRunQuotesArgvAndPrefixesSudoForNonRoot(t *testing.T) {
	node := startFakeNode(t, "terraform")
	node.reply = func(string) (string, uint32) { return "ok\n", 0 }
	r, err := New(node.config("terraform"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	out, err := r.Run(context.Background(), "pve01", []string{"/usr/sbin/pveceph", "install", "--version", "it's; rm -rf /"}, "y\n")
	if err != nil {
		t.Fatal(err)
	}
	if out.Stdout != "ok\n" {
		t.Fatalf("stdout = %q", out.Stdout)
	}
	want := `sudo -n '/usr/sbin/pveceph' 'install' '--version' 'it'\''s; rm -rf /'`
	if got := node.commands[0]; got != want {
		t.Fatalf("remote command\n got: %s\nwant: %s", got, want)
	}
	if node.stdins[0] != "y\n" {
		t.Fatalf("stdin = %q", node.stdins[0])
	}
}

func TestRunRootSkipsSudoAndReportsFailureWithStderr(t *testing.T) {
	node := startFakeNode(t, "root")
	node.reply = func(string) (string, uint32) { return "E: unable to locate package\n", 100 }
	r, err := New(node.config("root"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, err = r.Run(context.Background(), "pve01", []string{"/usr/sbin/pveceph", "install"}, "")
	if err == nil || !strings.Contains(err.Error(), "unable to locate package") {
		t.Fatalf("want stderr in error, got %v", err)
	}
	if strings.HasPrefix(node.commands[0], "sudo") {
		t.Fatalf("root must not use sudo: %s", node.commands[0])
	}
}

func TestRootTicketIsMintedOnceAndCached(t *testing.T) {
	node := startFakeNode(t, "root")
	node.reply = func(string) (string, uint32) { return "PVE:root@pam:ABC::sig\n12345678:csrf\n", 0 }
	cfg := node.config("root")
	cfg.RootTicket = true
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for range 3 {
		ticket, csrf, err := r.RootTicket(context.Background(), "pve01")
		if err != nil {
			t.Fatal(err)
		}
		if ticket != "PVE:root@pam:ABC::sig" || csrf != "12345678:csrf" {
			t.Fatalf("ticket=%q csrf=%q", ticket, csrf)
		}
	}
	if len(node.commands) != 1 {
		t.Fatalf("ticket minted %d times, want 1", len(node.commands))
	}
}

func TestForgetRootTicketForcesRemint(t *testing.T) {
	node := startFakeNode(t, "root")
	node.reply = func(string) (string, uint32) { return "PVE:root@pam:ABC::sig\n12345678:csrf\n", 0 }
	cfg := node.config("root")
	cfg.RootTicket = true
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, _, err := r.RootTicket(context.Background(), "pve01"); err != nil {
		t.Fatal(err)
	}
	r.ForgetRootTicket("pve01")
	if _, _, err := r.RootTicket(context.Background(), "pve01"); err != nil {
		t.Fatal(err)
	}
	if len(node.commands) != 2 {
		t.Fatalf("ticket minted %d times, want 2", len(node.commands))
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no user":                 {Password: "x"},
		"no auth":                 {Username: "root"},
		"root ticket w/ non-root": {Username: "terraform", Password: "x", RootTicket: true},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestHostKeyVerification(t *testing.T) {
	node := startFakeNode(t, "root")
	host, port, _ := net.SplitHostPort(node.addr)
	dir := t.TempDir()

	// A known_hosts entry for a different key must be rejected.
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPriv)
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte(knownhosts.Line([]string{net.JoinHostPort(host, port)}, otherSigner.PublicKey())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := node.config("root")
	cfg.InsecureIgnoreHostKey = false
	cfg.KnownHostsFile = bad
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), "pve01", []string{"true"}, ""); err == nil {
		t.Fatal("mismatching host key must fail")
	}

	// The right key connects.
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, []byte(knownhosts.Line([]string{net.JoinHostPort(host, port)}, node.hostKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.KnownHostsFile = good
	r2, _ := New(cfg)
	defer r2.Close()
	if _, err := r2.Run(context.Background(), "pve01", []string{"true"}, ""); err != nil {
		t.Fatalf("matching host key: %v", err)
	}
}
