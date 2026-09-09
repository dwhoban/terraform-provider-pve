// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

// Package credentials resolves Proxmox VE credentials from a chain of sources:
// static provider configuration, environment variables, and an optional
// credentials file with named profiles. Two credential shapes are supported:
// an API token (USER@REALM!TOKENID=UUID) sent in an Authorization header, or
// a username/password pair that obtains a session ticket via /access/ticket.
package credentials

import (
	"context"
	"errors"
	"strings"
)

// Sentinel returned by Provider.Retrieve when the source has nothing to
// contribute. Wrapped by ChainError when every source falls through cleanly.
var ErrNoCredentials = errors.New("no credentials found")

// Credentials is the resolved set of values needed to talk to the PVE API.
// Source is the human-readable name of the provider that supplied the
// non-empty fields (e.g. "static configuration", "environment", "credentials
// file"). Secret fields (Token, Password, OTP) are redacted by String and
// GoString.
type Credentials struct {
	Token    string
	Username string
	Password string
	Endpoint string
	Insecure string
	RootCA   string
	OTP      string
	Source   string
}

// Complete reports whether the resolved set has enough material to call the
// API. A token is complete on its own; a username+password pair is complete
// only when both are non-empty. Endpoint and Insecure are independent
// configuration and do not affect completeness.
func (c Credentials) Complete() bool {
	if c.Token != "" {
		return true
	}
	return c.Username != "" && c.Password != ""
}

// redacted returns a copy of c with secrets replaced by a placeholder. Used
// by String and GoString; tests assert that no secret value is reachable
// through %v or %+v.
func (c Credentials) redacted() Credentials {
	cp := c
	if cp.Token != "" {
		cp.Token = "***REDACTED***"
	}
	if cp.Password != "" {
		cp.Password = "***REDACTED***"
	}
	if cp.OTP != "" {
		cp.OTP = "***REDACTED***"
	}
	return cp
}

// String formats Credentials for human consumption, redacting secrets.
func (c Credentials) String() string {
	r := c.redacted()
	var b strings.Builder
	b.WriteString("Credentials{Source:")
	b.WriteString(r.Source)
	if r.Token != "" {
		b.WriteString(", Token:***REDACTED***")
	}
	if r.Username != "" {
		b.WriteString(", Username:")
		b.WriteString(r.Username)
	}
	if r.Password != "" {
		b.WriteString(", Password:***REDACTED***")
	}
	if r.Endpoint != "" {
		b.WriteString(", Endpoint:")
		b.WriteString(r.Endpoint)
	}
	if r.Insecure != "" {
		b.WriteString(", Insecure:")
		b.WriteString(r.Insecure)
	}
	if r.RootCA != "" {
		b.WriteString(", RootCA:<")
		b.WriteString(itoa(len(r.RootCA)))
		b.WriteString(" bytes>")
	}
	if r.OTP != "" {
		b.WriteString(", OTP:***REDACTED***")
	}
	b.WriteString("}")
	return b.String()
}

// GoString formats Credentials for %v and %+v, redacting secrets.
func (c Credentials) GoString() string {
	r := c.redacted()
	var b strings.Builder
	b.WriteString("credentials.Credentials{Token:\"")
	b.WriteString(r.Token)
	b.WriteString("\", Username:\"")
	b.WriteString(r.Username)
	b.WriteString("\", Password:\"")
	b.WriteString(r.Password)
	b.WriteString("\", Endpoint:\"")
	b.WriteString(r.Endpoint)
	b.WriteString("\", Insecure:\"")
	b.WriteString(r.Insecure)
	b.WriteString("\", RootCA:\"")
	b.WriteString(r.RootCA)
	b.WriteString("\", OTP:\"")
	b.WriteString(r.OTP)
	b.WriteString("\", Source:\"")
	b.WriteString(r.Source)
	b.WriteString("\"}")
	return b.String()
}

// Provider yields Credentials from a single source. Retrieve returns
// ErrNoCredentials when the source has nothing to contribute; any other
// error is treated as a hard failure that stops the chain. Name returns a
// short human-readable label used in diagnostics.
type Provider interface {
	Retrieve(ctx context.Context) (Credentials, error)
	Name() string
}

// itoa avoids strconv.Itoa to keep this file dependency-free aside from
// the strings package.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
