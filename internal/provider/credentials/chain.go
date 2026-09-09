// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SkippedSource records one provider that the chain walked past: its name
// and why it did not contribute. A reason of "no credentials" means the
// source was empty (and the chain keeps walking); any other reason is a hard
// failure that stops the chain.
type SkippedSource struct {
	Name   string
	Reason error
}

// ChainError aggregates the outcome of every Provider the chain tried.
// errors.Is(err, ErrNoCredentials) is true only when every provider's
// reason is ErrNoCredentials; if any provider returned a hard error,
// errors.Is returns false and the wrapped cause is exposed via Unwrap.
type ChainError struct {
	Skipped []SkippedSource
}

// Error renders the chain outcome as a multi-line message suitable for a
// Terraform diagnostic detail.
func (e *ChainError) Error() string {
	if len(e.Skipped) == 0 {
		return "no credential sources configured"
	}
	var b strings.Builder
	b.WriteString("no complete credentials set found after trying ")
	b.WriteString(itoa(len(e.Skipped)))
	b.WriteString(" source(s):")
	for _, s := range e.Skipped {
		b.WriteString("\n  - ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		b.WriteString(s.Reason.Error())
	}
	return b.String()
}

// Is reports whether the chain fell through cleanly (every source returned
// ErrNoCredentials).
func (e *ChainError) Is(target error) bool {
	if target == nil {
		return false
	}
	if !errors.Is(target, ErrNoCredentials) {
		return false
	}
	for _, s := range e.Skipped {
		if !errors.Is(s.Reason, ErrNoCredentials) {
			return false
		}
	}
	return true
}

// Unwrap exposes the first hard error so callers can errors.Is on it. When
// every source fell through cleanly, Unwrap returns ErrNoCredentials and
// ChainError.Is(ErrNoCredentials) reports true.
func (e *ChainError) Unwrap() error {
	for _, s := range e.Skipped {
		if !errors.Is(s.Reason, ErrNoCredentials) {
			return s.Reason
		}
	}
	return ErrNoCredentials
}

// Chain walks a fixed list of Providers in order and returns the first
// complete Credentials set. A source that returns ErrNoCredentials is
// skipped; any other error stops the chain immediately and is returned
// wrapped in a *ChainError that records the partial progress.
type Chain struct {
	providers []Provider
}

// NewChain builds a Chain from the given providers. Order matters: earlier
// entries win when multiple sources could supply complete credentials.
func NewChain(providers ...Provider) *Chain {
	return &Chain{providers: append([]Provider(nil), providers...)}
}

// Providers returns the providers in evaluation order. Used by tests.
func (c *Chain) Providers() []Provider {
	return append([]Provider(nil), c.providers...)
}

// Retrieve implements Provider by walking the chain. The returned
// Credentials.Source is set to the winning provider's Name. If no source
// yields a complete set, the returned error wraps ErrNoCredentials and is
// safe to errors.Is.
func (c *Chain) Retrieve(ctx context.Context) (Credentials, error) {
	var skipped []SkippedSource
	for _, p := range c.providers {
		creds, err := p.Retrieve(ctx)
		if err == nil {
			if creds.Source == "" {
				creds.Source = p.Name()
			}
			if creds.Complete() {
				return creds, nil
			}
			skipped = append(skipped, SkippedSource{Name: p.Name(), Reason: ErrNoCredentials})
			continue
		}
		if errors.Is(err, ErrNoCredentials) {
			skipped = append(skipped, SkippedSource{Name: p.Name(), Reason: err})
			continue
		}
		// Hard error: stop and surface the wrapped failure. The skipped
		// entries up to this point are still useful for diagnostics.
		skipped = append(skipped, SkippedSource{Name: p.Name(), Reason: err})
		return Credentials{}, &ChainError{Skipped: skipped}
	}
	return Credentials{}, &ChainError{Skipped: skipped}
}

// Name satisfies Provider so a Chain can itself be a member of another
// Chain. It is unusual to nest chains, but the interface compliance makes
// composition trivial.
func (c *Chain) Name() string { return fmt.Sprintf("chain(%d sources)", len(c.providers)) }
