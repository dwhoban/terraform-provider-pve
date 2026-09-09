// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Env keys. Keep these as constants so the test harness can assert they are
// exactly the values documented for users.
const (
	EnvEndpoint        = "PROXMOX_VE_ENDPOINT"
	EnvAPIToken        = "PROXMOX_VE_API_TOKEN"
	EnvUsername        = "PROXMOX_VE_USERNAME"
	EnvPassword        = "PROXMOX_VE_PASSWORD"
	EnvInsecure        = "PROXMOX_VE_INSECURE"
	EnvRootCA          = "PROXMOX_VE_ROOT_CA"
	EnvOTP             = "PROXMOX_VE_OTP"
	EnvCredentialsFile = "PROXMOX_VE_CREDENTIALS_FILE"
)

// EnvProvider reads credentials from environment variables. GetEnv is
// injectable so tests can use t.Setenv without racing other parallel tests.
type EnvProvider struct {
	GetEnv func(string) string
}

// NewEnvProvider returns an EnvProvider backed by os.Getenv.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{GetEnv: os.Getenv}
}

// Retrieve reads the seven credential-related environment variables and
// returns Credentials or ErrNoCredentials when no credential field is set.
// A non-boolean, non-empty PROXMOX_VE_INSECURE value is treated as a hard
// error rather than a silent fallback, because misconfiguration there would
// silently weaken TLS.
func (p *EnvProvider) Retrieve(_ context.Context) (Credentials, error) {
	if p.GetEnv == nil {
		p.GetEnv = os.Getenv
	}
	insecure := p.GetEnv(EnvInsecure)
	if insecure != "" && !parseBool(insecure) {
		return Credentials{}, fmt.Errorf("%s=%q is not a valid boolean (expected \"true\" or \"1\")", EnvInsecure, insecure)
	}
	c := Credentials{
		Token:    p.GetEnv(EnvAPIToken),
		Username: p.GetEnv(EnvUsername),
		Password: p.GetEnv(EnvPassword),
		Endpoint: p.GetEnv(EnvEndpoint),
		Insecure: insecure,
		RootCA:   p.GetEnv(EnvRootCA),
		OTP:      p.GetEnv(EnvOTP),
		Source:   p.Name(),
	}
	if !c.Complete() {
		return Credentials{}, errors.Join(ErrNoCredentials, errors.New("no credential environment variables set"))
	}
	return c, nil
}

// Name returns the environment source label.
func (p *EnvProvider) Name() string { return "environment" }

// parseBool accepts the same boolean spellings as the standard library's
// strconv.ParseBool without pulling strconv into this file's import set.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes":
		return true
	default:
		return false
	}
}
