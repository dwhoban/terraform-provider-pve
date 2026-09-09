// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import "os"

// Options tunes the default chain. Zero values pick sensible defaults:
// Profile defaults to "default" for the FileProvider, and GetEnv defaults
// to os.Getenv.
type Options struct {
	// Profile selects which [profile] section the FileProvider reads.
	Profile string
	// CredentialsFile overrides the FileProvider's resolved path. Useful
	// for tests; production code should rely on the PROXMOX_VE_CREDENTIALS_FILE
	// env var instead.
	CredentialsFile string
	// GetEnv is the environment reader. Tests inject a custom function so
	// t.Setenv races do not affect parallel tests.
	GetEnv func(string) string
}

// NewDefaultChain returns a Chain that walks static configuration, then the
// environment, then the credentials file, in that order. The static
// provider is built from the supplied Credentials (typically populated from
// the Terraform provider block).
func NewDefaultChain(static Credentials, opts Options) *Chain {
	getEnv := opts.GetEnv
	if getEnv == nil {
		getEnv = os.Getenv
	}
	profile := opts.Profile
	if profile == "" {
		profile = "default"
	}
	file := NewFileProvider(profile)
	file.GetEnv = getEnv
	if opts.CredentialsFile != "" {
		file.Path = opts.CredentialsFile
	}
	env := &EnvProvider{GetEnv: getEnv}
	return NewChain(NewStaticProvider(static), env, file)
}
