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
	APIToken   string
	APITokenID string
	APISecret  string
	Username   string
	Password   string
	TOTP       string
	Getenv     func(string) string
}

const credentialDocsHint = "Set api_token (user@realm!tokenid=uuid), or api_token_id and api_secret, or username and password, in the provider block; or export PVE_API_TOKEN, or PVE_API_TOKEN_ID and PVE_API_SECRET, or PVE_USER and PVE_PASSWORD. See the provider authentication documentation in https://registry.terraform.io/dwhoban/pve/latest/docs."

// ResolveCredentials resolves credentials by consulting, in order:
//
//  1. static provider configuration
//  2. environment variables (PVE_API_TOKEN, or PVE_API_TOKEN_ID + PVE_API_SECRET, or PVE_USER + PVE_PASSWORD, with optional PVE_TOTP)
//
// Each source must yield a complete set — a token (combined, or split into
// id + secret), or both username and password. A partially populated source
// is a hard error naming the missing fields, because silently mixing
// sources produces wrong-identity authentication failures; mixing the
// combined and split token forms within one source is equally a hard error.
// When every source is empty, the returned error wraps ErrNoCredentials and
// names each source tried.
func ResolveCredentials(opts ResolveOptions) (Credentials, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}

	// Source 1: static provider configuration.
	if opts.APIToken != "" || opts.APITokenID != "" || opts.APISecret != "" || opts.Username != "" || opts.Password != "" {
		token, err := normalizeTokenForm(opts.APIToken, opts.APITokenID, opts.APISecret,
			"provider configuration", "api_token", "api_token_id", "api_secret")
		if err != nil {
			return Credentials{}, err
		}
		creds := Credentials{
			APIToken: token,
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
	envTokenID := opts.Getenv(EnvAPITokenID)
	envSecret := opts.Getenv(EnvAPISecret)
	envUser := opts.Getenv(EnvUser)
	envPassword := opts.Getenv(EnvPassword)
	if envToken != "" || envTokenID != "" || envSecret != "" || envUser != "" || envPassword != "" {
		token, err := normalizeTokenForm(envToken, envTokenID, envSecret,
			"environment variables", EnvAPIToken, EnvAPITokenID, EnvAPISecret)
		if err != nil {
			return Credentials{}, err
		}
		creds := Credentials{
			APIToken: token,
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

// normalizeTokenForm folds the split token credentials (id + secret) into
// the combined "<id>=<secret>" form used on the wire. Within a single
// source, the combined and split forms are mutually exclusive and a half
// pair is a hard error naming the missing field.
func normalizeTokenForm(combined, id, secret string, sourceLabel, combinedName, idName, secretName string) (string, error) {
	switch {
	case combined != "":
		if id != "" || secret != "" {
			return "", fmt.Errorf("conflicting credentials in %s: %s cannot be combined with %s/%s. Set either %s, or %s and %s: %s",
				sourceLabel, combinedName, idName, secretName, combinedName, idName, secretName, credentialDocsHint)
		}
		return combined, nil
	case id != "" && secret != "":
		return id + "=" + secret, nil
	case id != "":
		return "", fmt.Errorf("incomplete credentials in %s (missing %s): %s", sourceLabel, secretName, credentialDocsHint)
	case secret != "":
		return "", fmt.Errorf("incomplete credentials in %s (missing %s): %s", sourceLabel, idName, credentialDocsHint)
	default:
		return "", nil
	}
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
