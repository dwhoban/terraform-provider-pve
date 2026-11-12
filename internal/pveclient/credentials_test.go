// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveCredentials_staticToken(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{APIToken: "root@pam!t=secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !creds.Complete() || !creds.UsesToken() {
		t.Fatalf("expected complete token credentials, got %+v", creds)
	}
	if creds.Source != "provider configuration" {
		t.Errorf("Source = %q", creds.Source)
	}
}

func TestResolveCredentials_staticUserPassword(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{Username: "root@pam", Password: "hunter2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.UsesToken() {
		t.Fatal("expected ticket credentials")
	}
}

func TestResolveCredentials_staticIncompleteIsHardError(t *testing.T) {
	_, err := ResolveCredentials(ResolveOptions{Username: "root@pam"})
	if err == nil {
		t.Fatal("expected error for incomplete static set")
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Error("incomplete static set must not wrap ErrNoCredentials")
	}
	if !strings.Contains(err.Error(), "provider configuration") {
		t.Errorf("error should name the source: %v", err)
	}
}

func TestResolveCredentials_envToken(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{Getenv: func(k string) string {
		if k == EnvAPIToken {
			return "root@pam!t=secret"
		}
		return ""
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !creds.UsesToken() || creds.Source != "environment variables" {
		t.Fatalf("got %+v", creds)
	}
}

func TestResolveCredentials_envUserPassword(t *testing.T) {
	env := map[string]string{EnvUser: "root@pam", EnvPassword: "hunter2", EnvTOTP: "123456"}
	creds, err := ResolveCredentials(ResolveOptions{Getenv: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Username != "root@pam" || creds.Password != "hunter2" || creds.TOTP != "123456" {
		t.Fatalf("got %+v", creds)
	}
}

func TestResolveCredentials_envIncompleteIsHardError(t *testing.T) {
	_, err := ResolveCredentials(ResolveOptions{Getenv: func(k string) string {
		if k == EnvUser {
			return "root@pam"
		}
		return ""
	}})
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Fatalf("expected hard error for partial env, got %v", err)
	}
}

func TestResolveCredentials_noneWrapsErrNoCredentials(t *testing.T) {
	_, err := ResolveCredentials(ResolveOptions{Getenv: func(string) string { return "" }})
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
	for _, want := range []string{"provider configuration", "environment variables", EnvAPIToken, EnvUser, EnvPassword} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestCredentialsRedacted(t *testing.T) {
	c := Credentials{APIToken: "super-secret", Source: "env"}
	for _, s := range []string{c.String(), c.GoString()} {
		if strings.Contains(s, "super-secret") {
			t.Errorf("token secret leaked: %s", s)
		}
	}
	c2 := Credentials{Username: "root@pam", Password: "hunter2", TOTP: "123456"}
	for _, s := range []string{c2.String(), c2.GoString()} {
		if strings.Contains(s, "hunter2") || strings.Contains(s, "123456") {
			t.Errorf("secret leaked: %s", s)
		}
	}
}

func TestResolveCredentials_staticBeatsEnv(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{
		APIToken: "root@pam!t=static",
		Getenv: func(k string) string {
			if k == EnvUser {
				return "root@pam"
			}
			if k == EnvPassword {
				return "hunter2"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if !creds.UsesToken() {
		t.Fatalf("static token must take precedence over environment user/password")
	}
	if creds.Source != "provider configuration" {
		t.Fatalf("source = %q, want %q", creds.Source, "provider configuration")
	}
}

func TestResolveCredentials_staticSplitToken(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{
		APITokenID: "root@pam!terraform",
		APISecret:  "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIToken != "root@pam!terraform=00000000-0000-0000-0000-000000000000" {
		t.Errorf("APIToken = %q", creds.APIToken)
	}
	if !creds.Complete() || !creds.UsesToken() {
		t.Fatalf("expected complete token credentials, got %+v", creds)
	}
	if creds.Source != "provider configuration" {
		t.Errorf("Source = %q", creds.Source)
	}
}

func TestResolveCredentials_staticSplitConflict(t *testing.T) {
	_, err := ResolveCredentials(ResolveOptions{
		APIToken:   "root@pam!t=secret",
		APITokenID: "root@pam!t",
	})
	if err == nil {
		t.Fatal("expected error for combined plus split token")
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Error("conflict must be a hard error, not ErrNoCredentials")
	}
	if !strings.Contains(err.Error(), "api_token cannot be combined with api_token_id/api_secret") {
		t.Errorf("error should name the conflicting attributes: %v", err)
	}
}

func TestResolveCredentials_staticSplitIncomplete(t *testing.T) {
	_, err := ResolveCredentials(ResolveOptions{APITokenID: "root@pam!terraform"})
	if err == nil || !strings.Contains(err.Error(), "api_secret") {
		t.Fatalf("expected missing api_secret error, got %v", err)
	}
	_, err = ResolveCredentials(ResolveOptions{APISecret: "00000000-0000-0000-0000-000000000000"})
	if err == nil || !strings.Contains(err.Error(), "api_token_id") {
		t.Fatalf("expected missing api_token_id error, got %v", err)
	}
}

func TestResolveCredentials_envSplitToken(t *testing.T) {
	env := map[string]string{
		EnvAPITokenID: "root@pam!terraform",
		EnvAPISecret:  "00000000-0000-0000-0000-000000000000",
	}
	creds, err := ResolveCredentials(ResolveOptions{Getenv: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIToken != "root@pam!terraform=00000000-0000-0000-0000-000000000000" || !creds.UsesToken() {
		t.Fatalf("got %+v", creds)
	}
	if creds.Source != "environment variables" {
		t.Errorf("Source = %q", creds.Source)
	}
}

func TestResolveCredentials_staticCombinedBeatsEnvSplit(t *testing.T) {
	creds, err := ResolveCredentials(ResolveOptions{
		APIToken: "root@pam!t=static",
		Getenv: func(k string) string {
			if k == EnvAPITokenID {
				return "root@pam!t"
			}
			if k == EnvAPISecret {
				return "env-secret"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.APIToken != "root@pam!t=static" || !creds.UsesToken() {
		t.Fatalf("static combined token must take precedence over environment split pair, got %+v", creds)
	}
	if creds.Source != "provider configuration" {
		t.Errorf("source = %q, want %q", creds.Source, "provider configuration")
	}
}
