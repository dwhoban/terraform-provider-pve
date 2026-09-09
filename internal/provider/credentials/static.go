// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import (
	"context"
	"errors"
)

// StaticProvider supplies credentials from values passed directly to the
// provider block (the highest-precedence source in the chain). All fields
// are optional; if none of the credential fields are set, Retrieve returns
// ErrNoCredentials so the chain falls through to the next source.
type StaticProvider struct {
	Token    string
	Username string
	Password string
	Endpoint string
	Insecure string
	RootCA   string
	OTP      string
}

// NewStaticProvider constructs a StaticProvider from a Credentials value.
// The Source field is ignored; StaticProvider always reports itself as
// "static configuration".
func NewStaticProvider(c Credentials) *StaticProvider {
	return &StaticProvider{
		Token:    c.Token,
		Username: c.Username,
		Password: c.Password,
		Endpoint: c.Endpoint,
		Insecure: c.Insecure,
		RootCA:   c.RootCA,
		OTP:      c.OTP,
	}
}

// Retrieve returns the configured credentials or ErrNoCredentials when no
// credential field is set.
func (p *StaticProvider) Retrieve(_ context.Context) (Credentials, error) {
	c := Credentials{
		Token:    p.Token,
		Username: p.Username,
		Password: p.Password,
		Endpoint: p.Endpoint,
		Insecure: p.Insecure,
		RootCA:   p.RootCA,
		OTP:      p.OTP,
		Source:   p.Name(),
	}
	if !c.Complete() {
		return Credentials{}, errors.Join(ErrNoCredentials, errors.New("no credentials set in provider block"))
	}
	return c, nil
}

// Name returns the static source label.
func (p *StaticProvider) Name() string { return "static configuration" }
