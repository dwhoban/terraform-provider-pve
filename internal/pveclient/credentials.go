// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNoCredentials signals that no complete credential set was found in any
// source. Errors wrapping it are produced only when every source was empty;
// a source that was partially populated is a hard error instead.
var ErrNoCredentials = errors.New("no valid credential sources found")

// Credentials is a resolved Proxmox VE credential set: either an API token
// (complete on its own) or a username/password pair used to obtain a session
// ticket. TOTP is optional and only used with username/password.
type Credentials struct {
	APIToken string
	Username string
	Password string
	TOTP     string

	// Source names where the set came from, for logging and diagnostics.
	Source string
}

// Complete reports whether the set can authenticate: an API token alone, or
// both username and password.
func (c Credentials) Complete() bool {
	return c.APIToken != "" || (c.Username != "" && c.Password != "")
}

// UsesToken reports whether authentication uses a static API token rather
// than a username/password ticket.
func (c Credentials) UsesToken() bool { return c.APIToken != "" }

// String redacts secrets so a stray %v/%s can never leak them.
func (c Credentials) String() string {
	if c.UsesToken() {
		return fmt.Sprintf("Credentials{APIToken:<redacted>, Source:%q}", c.Source)
	}
	return fmt.Sprintf("Credentials{Username:%q, Password:<redacted>, TOTP:<redacted>, Source:%q}", c.Username, c.Source)
}

// GoString implements fmt.GoStringer and redacts like String.
func (c Credentials) GoString() string { return c.String() }

// ResolveOptions carries static provider-configuration values into
// ResolveCredentials. Getenv is injectable for tests and defaults to
// os.Getenv.
type ResolveOptions struct {
	APIToken string
	Username string
	Password string
	TOTP     string
	Getenv   func(string) string
}

const credentialDocsHint = "Set api_token, or username and password, in the provider block; or export PVE_API_TOKEN, or PVE_USER and PVE_PASSWORD. See the provider authentication documentation in docs/index.md."

// ResolveCredentials resolves credentials by consulting, in order:
//
//  1. static provider configuration
//  2. environment variables (PVE_API_TOKEN, or PVE_USER + PVE_PASSWORD, with optional PVE_TOTP)
//
// Each source must yield a complete set — a token, or both username and
// password. A partially populated source is a hard error naming the missing
// fields, because silently mixing sources produces wrong-identity
// authentication failures. When every source is empty, the returned error
// wraps ErrNoCredentials and names each source tried.
func ResolveCredentials(opts ResolveOptions) (Credentials, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}

	// Source 1: static provider configuration.
	if opts.APIToken != "" || opts.Username != "" || opts.Password != "" {
		creds := Credentials{
			APIToken: opts.APIToken,
			Username: opts.Username,
			Password: opts.Password,
			TOTP:     opts.TOTP,
			Source:   "provider configuration",
		}
		if creds.Complete() {
			return creds, nil
		}
		return Credentials{}, fmt.Errorf("incomplete credentials in provider configuration (%s): %s", missingFields(creds), credentialDocsHint)
	}

	// Source 2: environment variables.
	envToken := opts.Getenv(EnvAPIToken)
	envUser := opts.Getenv(EnvUser)
	envPassword := opts.Getenv(EnvPassword)
	if envToken != "" || envUser != "" || envPassword != "" {
		creds := Credentials{
			APIToken: envToken,
			Username: envUser,
			Password: envPassword,
			TOTP:     opts.Getenv(EnvTOTP),
			Source:   "environment variables",
		}
		if creds.Complete() {
			return creds, nil
		}
		return Credentials{}, fmt.Errorf("incomplete credentials in environment variables (%s): %s", missingFields(creds), credentialDocsHint)
	}

	return Credentials{}, fmt.Errorf("%w: sources tried: provider configuration (empty), environment variables %s/%s/%s (empty): %s",
		ErrNoCredentials, EnvAPIToken, EnvUser, EnvPassword, credentialDocsHint)
}

func missingFields(c Credentials) string {
	var missing []string
	if c.APIToken == "" && c.Username == "" && c.Password == "" {
		return "no fields set"
	}
	if c.APIToken == "" && c.Username == "" {
		missing = append(missing, "api_token or username")
	}
	if c.APIToken == "" && c.Password == "" {
		missing = append(missing, "api_token or password")
	}
	return "missing " + strings.Join(missing, ", ")
}
