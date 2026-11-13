// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

// Package pvessh runs commands on Proxmox VE nodes over SSH.
//
// The provider is API-first; SSH exists only for the few operations the PVE
// API cannot perform (installing Ceph packages) and for minting a root@pam
// ticket so an API token can perform root@pam-only calls. Connections are
// opened lazily, per node, so a configuration that never needs SSH never
// dials. Commands are always argv vectors: every argument is single-quoted
// before it reaches the remote shell, so a strict sudoers rule can match the
// exact command line.
package pvessh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	defaultPort = 22
	dialTimeout = 15 * time.Second
	// ticketLifetime renews a cached ticket well before PVE's two-hour
	// validity can run out mid-apply.
	ticketLifetime = time.Hour
)

// Node pins the SSH address of one node.
type Node struct {
	Name    string
	Address string
	Port    int
}

// Config is the resolved ssh block.
type Config struct {
	Username   string
	Password   string
	PrivateKey string // PEM, unencrypted
	Agent      bool
	AgentSock  string // defaults to $SSH_AUTH_SOCK

	// KnownHostsFile verifies host keys (default ~/.ssh/known_hosts).
	KnownHostsFile string
	// InsecureIgnoreHostKey disables host key verification.
	InsecureIgnoreHostKey bool

	// RootTicket mints root@pam tickets for API calls under /nodes/{node}/.
	RootTicket bool

	Nodes []Node
}

// Resolver finds the SSH address of a node that has no explicit entry.
type Resolver func(ctx context.Context, node string) (string, error)

// Runner executes commands on nodes. It implements pveclient.Shell and
// pveclient.RootTicketSource.
type Runner struct {
	cfg      Config
	resolver Resolver

	mu      sync.Mutex
	clients map[string]*ssh.Client
	addrs   map[string]string
	tickets map[string]cachedTicket

	// dial is replaceable in tests.
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

type cachedTicket struct {
	ticket, csrf string
	minted       time.Time
}

var (
	_ pveclient.Shell            = (*Runner)(nil)
	_ pveclient.RootTicketSource = (*Runner)(nil)
)

// New validates cfg and returns a Runner. No connection is made.
func New(cfg Config) (*Runner, error) {
	if cfg.Username == "" {
		return nil, errors.New("ssh: username is required (set ssh.username or PVE_SSH_USERNAME; with an API token the PAM user cannot be inferred)")
	}
	if cfg.Password == "" && cfg.PrivateKey == "" && !cfg.Agent {
		return nil, errors.New("ssh: no authentication method — set ssh.agent, ssh.private_key or ssh.password")
	}
	if cfg.RootTicket && cfg.Username != "root" {
		return nil, errors.New("ssh: root_ticket requires ssh.username = \"root\" (minting a root@pam ticket needs a root shell)")
	}
	d := net.Dialer{Timeout: dialTimeout}
	return &Runner{
		cfg:     cfg,
		clients: map[string]*ssh.Client{},
		addrs:   map[string]string{},
		tickets: map[string]cachedTicket{},
		dial:    d.DialContext,
	}, nil
}

// RootTicketEnabled reports whether root@pam ticket minting is configured.
func (r *Runner) RootTicketEnabled() bool { return r.cfg.RootTicket }

// SetResolver installs the fallback node address lookup.
func (r *Runner) SetResolver(fn Resolver) { r.resolver = fn }

// Close drops every open connection.
func (r *Runner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, c := range r.clients {
		_ = c.Close()
		delete(r.clients, k)
	}
}

// Quote single-quotes one argument for a POSIX shell.
func Quote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// CommandLine renders argv as the exact remote command line. Non-root users
// run through `sudo -n` (never prompting), so sudoers rules must match the
// quoted line.
func (r *Runner) CommandLine(argv []string) string {
	parts := make([]string, 0, len(argv)+2)
	if r.cfg.Username != "root" {
		parts = append(parts, "sudo", "-n")
	}
	for _, a := range argv {
		parts = append(parts, Quote(a))
	}
	return strings.Join(parts, " ")
}

// Run executes argv on node and returns its output. A non-zero exit status
// is an error that carries the trimmed stderr.
func (r *Runner) Run(ctx context.Context, node string, argv []string, stdin string) (pveclient.ShellOutput, error) {
	return r.run(ctx, node, r.CommandLine(argv), stdin)
}

func (r *Runner) run(ctx context.Context, node, cmdline, stdin string) (pveclient.ShellOutput, error) {
	client, err := r.connect(ctx, node)
	if err != nil {
		return pveclient.ShellOutput{}, err
	}
	sess, err := client.NewSession()
	if err != nil {
		r.drop(node, client)
		return pveclient.ShellOutput{}, fmt.Errorf("ssh %s: opening session: %w", node, err)
	}
	defer sess.Close() //nolint:errcheck
	var stdout, stderr bytes.Buffer
	sess.Stdout, sess.Stderr = &stdout, &stderr
	if stdin != "" {
		sess.Stdin = strings.NewReader(stdin)
	}

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmdline) }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return pveclient.ShellOutput{}, fmt.Errorf("ssh %s: %w", node, ctx.Err())
	case err = <-done:
	}
	out := pveclient.ShellOutput{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		msg := strings.TrimSpace(out.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(out.Stdout)
		}
		return out, fmt.Errorf("ssh %s: %w: %s", node, err, tail(msg, 2000))
	}
	return out, nil
}

// RootTicket mints (and caches for an hour) a root@pam ticket on node.
func (r *Runner) RootTicket(ctx context.Context, node string) (string, string, error) {
	if !r.cfg.RootTicket {
		return "", "", errors.New("ssh.root_ticket is not enabled")
	}
	r.mu.Lock()
	t, ok := r.tickets[node]
	r.mu.Unlock()
	if ok && time.Since(t.minted) < ticketLifetime {
		return t.ticket, t.csrf, nil
	}
	const script = `use PVE::AccessControl; print PVE::AccessControl::assemble_ticket("root@pam"), "\n", PVE::AccessControl::assemble_csrf_prevention_token("root@pam"), "\n";`
	out, err := r.run(ctx, node, Quote("perl")+" -MPVE::AccessControl -e "+Quote(script), "")
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", errors.New("unexpected output while minting the root@pam ticket")
	}
	t = cachedTicket{ticket: strings.TrimSpace(lines[0]), csrf: strings.TrimSpace(lines[1]), minted: time.Now()}
	r.mu.Lock()
	r.tickets[node] = t
	r.mu.Unlock()
	return t.ticket, t.csrf, nil
}

func (r *Runner) drop(node string, c *ssh.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients[node] == c {
		delete(r.clients, node)
	}
	_ = c.Close()
}

func (r *Runner) connect(ctx context.Context, node string) (*ssh.Client, error) {
	r.mu.Lock()
	if c, ok := r.clients[node]; ok {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	addr := r.address(ctx, node)
	auth, closeAgent, err := r.authMethods()
	if err != nil {
		return nil, err
	}
	hostKey, err := r.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	conn, err := r.dial(ctx, "tcp", addr)
	if err != nil {
		if closeAgent != nil {
			closeAgent()
		}
		return nil, fmt.Errorf("ssh %s: dialing %s: %w", node, addr, err)
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User:            r.cfg.Username,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         dialTimeout,
	})
	if err != nil {
		_ = conn.Close()
		if closeAgent != nil {
			closeAgent()
		}
		return nil, fmt.Errorf("ssh %s (%s): %w", node, addr, err)
	}
	client := ssh.NewClient(sc, chans, reqs)
	r.mu.Lock()
	if existing, ok := r.clients[node]; ok { // lost a race
		r.mu.Unlock()
		_ = client.Close()
		return existing, nil
	}
	r.clients[node] = client
	r.mu.Unlock()
	return client, nil
}

func (r *Runner) address(ctx context.Context, node string) string {
	r.mu.Lock()
	cached, ok := r.addrs[node]
	r.mu.Unlock()
	if ok {
		return cached
	}
	port := defaultPort
	host := ""
	for _, n := range r.cfg.Nodes {
		if n.Name == node {
			host = n.Address
			if n.Port > 0 {
				port = n.Port
			}
			break
		}
	}
	if host == "" && r.resolver != nil {
		h, err := r.resolver(ctx, node)
		if err == nil {
			host = h
		}
	}
	if host == "" {
		host = node // last resort: DNS name of the node
	}
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	r.mu.Lock()
	r.addrs[node] = addr
	r.mu.Unlock()
	return addr
}

func (r *Runner) authMethods() ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	var closer func()
	if r.cfg.Agent {
		sock := r.cfg.AgentSock
		if sock == "" {
			sock = os.Getenv("SSH_AUTH_SOCK")
		}
		if sock == "" {
			return nil, nil, errors.New("ssh.agent is enabled but no agent socket is set (ssh.agent_socket / SSH_AUTH_SOCK)")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to ssh agent %s: %w", sock, err)
		}
		closer = func() { _ = conn.Close() }
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	}
	if r.cfg.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(r.cfg.PrivateKey))
		if err != nil {
			return nil, nil, fmt.Errorf("ssh.private_key: %w (must be an unencrypted PEM/OpenSSH key)", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if r.cfg.Password != "" {
		methods = append(methods, ssh.Password(r.cfg.Password))
	}
	return methods, closer, nil
}

func (r *Runner) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if r.cfg.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit opt-in
	}
	file := r.cfg.KnownHostsFile
	if file == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("ssh: cannot locate known_hosts: %w (set ssh.known_hosts_file or ssh.insecure_ignore_host_key)", err)
		}
		file = home + "/.ssh/known_hosts"
	}
	cb, err := knownhosts.New(file)
	if err != nil {
		return nil, fmt.Errorf("ssh: reading known_hosts %q: %w (add the node's host key, set ssh.known_hosts_file, or set ssh.insecure_ignore_host_key)", file, err)
	}
	return cb, nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
