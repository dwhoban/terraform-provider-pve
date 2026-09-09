// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStaticProvider_TokenComplete(t *testing.T) {
	p := NewStaticProvider(Credentials{Token: "root@pam!tf=abc", Endpoint: "https://pve.example.com:8006/"})
	creds, err := p.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !creds.Complete() {
		t.Fatal("Complete() returned false for token-only credentials")
	}
	if creds.Source != "static configuration" {
		t.Fatalf("Source = %q, want %q", creds.Source, "static configuration")
	}
	if creds.Token != "root@pam!tf=abc" {
		t.Fatalf("Token = %q, want it preserved", creds.Token)
	}
}

func TestStaticProvider_UserPassComplete(t *testing.T) {
	p := NewStaticProvider(Credentials{Username: "root@pam", Password: "hunter2"})
	creds, err := p.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !creds.Complete() {
		t.Fatal("Complete() returned false for username+password credentials")
	}
	if creds.Token != "" {
		t.Fatal("expected empty Token when only username/password supplied")
	}
}

func TestStaticProvider_PartialFallsThrough(t *testing.T) {
	p := NewStaticProvider(Credentials{Username: "root@pam"})
	_, err := p.Retrieve(context.Background())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want errors.Is(ErrNoCredentials) true", err)
	}
}

func TestStaticProvider_AllEmpty(t *testing.T) {
	p := NewStaticProvider(Credentials{})
	_, err := p.Retrieve(context.Background())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want errors.Is(ErrNoCredentials) true", err)
	}
}

func TestChain_Precedence_StaticBeatsEnv(t *testing.T) {
	static := Credentials{Token: "root@pam!static=AAAA"}
	env := map[string]string{EnvAPIToken: "root@pam!env=BBBB"}
	c := NewDefaultChain(static, Options{GetEnv: func(k string) string { return env[k] }})
	creds, err := c.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if creds.Source != "static configuration" {
		t.Fatalf("Source = %q, want static configuration", creds.Source)
	}
	if creds.Token != "root@pam!static=AAAA" {
		t.Fatalf("Token = %q, want static value", creds.Token)
	}
}

func TestChain_Precedence_EnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	writeFile(t, path, `[default]
api_token = root@pam!file=CCCC
`+"\n")
	env := map[string]string{
		EnvAPIToken:        "root@pam!env=DDDD",
		EnvCredentialsFile: path,
	}
	c := NewDefaultChain(Credentials{}, Options{GetEnv: func(k string) string { return env[k] }})
	creds, err := c.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if creds.Source != "environment" {
		t.Fatalf("Source = %q, want environment", creds.Source)
	}
	if creds.Token != "root@pam!env=DDDD" {
		t.Fatalf("Token = %q, want env value", creds.Token)
	}
}

func TestChain_AllEmpty_AggregatesAsNoCredentials(t *testing.T) {
	c := NewDefaultChain(Credentials{}, Options{GetEnv: func(string) string { return "" }})
	_, err := c.Retrieve(context.Background())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want errors.Is(ErrNoCredentials) true", err)
	}
	if !strings.Contains(err.Error(), "static configuration") {
		t.Fatalf("error message missing static source name: %v", err)
	}
	if !strings.Contains(err.Error(), "environment") {
		t.Fatalf("error message missing environment source name: %v", err)
	}
	if !strings.Contains(err.Error(), "credentials file") {
		t.Fatalf("error message missing credentials-file source name: %v", err)
	}
}

func TestChain_FileHardError_NotNoCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	writeFile(t, path, `[default]
weird = thing
`+"\n")
	c := NewDefaultChain(Credentials{}, Options{
		CredentialsFile: path,
		GetEnv:          func(string) string { return "" },
	})
	_, err := c.Retrieve(context.Background())
	if errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want hard error (not ErrNoCredentials)", err)
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("error message missing parse detail: %v", err)
	}
}

func TestCredentials_Redaction(t *testing.T) {
	c := Credentials{
		Token:    "supersecret-token",
		Password: "supersecret-password",
		OTP:      "supersecret-otp",
		Username: "root@pam",
		Endpoint: "https://pve.example.com:8006/",
	}
	for _, format := range []string{"%v", "%+v"} {
		s := fmt.Sprintf(format, c)
		for _, secret := range []string{"supersecret-token", "supersecret-password", "supersecret-otp"} {
			if strings.Contains(s, secret) {
				t.Fatalf("format %s leaked secret %q in output: %s", format, secret, s)
			}
		}
	}
	if !strings.Contains(c.String(), "***REDACTED***") {
		t.Fatalf("String() missing redaction marker: %s", c.String())
	}
}

func TestFileProvider_PermissionsWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions warning does not apply on Windows")
	}
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldDefault) })

	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	writeFile(t, path, "[default]\napi_token = root@pam!tf=xyz\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	p := NewFileProvider("default")
	p.Path = path
	if _, err := p.Retrieve(context.Background()); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !strings.Contains(buf.String(), "credentials file is readable by other users") {
		t.Fatalf("expected permissions warning in logs, got: %s", buf.String())
	}
}

func TestFileProvider_MalformedKeyBeforeSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	writeFile(t, path, "api_token = nope\n")
	p := NewFileProvider("default")
	p.Path = path
	_, err := p.Retrieve(context.Background())
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want hard parse error", err)
	}
}

func TestFileProvider_UnknownProfileFallsThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	writeFile(t, path, "[default]\napi_token = root@pam!tf=zzz\n")
	p := NewFileProvider("prod")
	p.Path = path
	_, err := p.Retrieve(context.Background())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want ErrNoCredentials for missing profile", err)
	}
}

func TestEnvProvider_BadInsecureIsHardError(t *testing.T) {
	p := &EnvProvider{GetEnv: func(k string) string {
		if k == EnvAPIToken {
			return "root@pam!tf=xyz"
		}
		if k == EnvInsecure {
			return "maybe"
		}
		return ""
	}}
	_, err := p.Retrieve(context.Background())
	if errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Retrieve err = %v, want hard error for bad insecure value", err)
	}
	if !strings.Contains(err.Error(), EnvInsecure) {
		t.Fatalf("error message missing env var name: %v", err)
	}
}

func TestEnvProvider_InsecureAcceptedValues(t *testing.T) {
	for _, val := range []string{"true", "TRUE", "1", "yes"} {
		p := &EnvProvider{GetEnv: func(k string) string {
			if k == EnvAPIToken {
				return "root@pam!tf=xyz"
			}
			if k == EnvInsecure {
				return val
			}
			return ""
		}}
		creds, err := p.Retrieve(context.Background())
		if err != nil {
			t.Fatalf("Retrieve for insecure=%q: %v", val, err)
		}
		if creds.Insecure != val {
			t.Fatalf("Insecure = %q, want %q", creds.Insecure, val)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
