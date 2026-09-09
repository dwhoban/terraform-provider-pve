// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package credentials

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileProvider parses an INI-style credentials file with [profile] sections.
// Path is resolved in this order: the explicit Path field, the
// PROXMOX_VE_CREDENTIALS_FILE environment variable, and
// ~/.proxmox/credentials (using os.UserHomeDir). Missing profile returns
// ErrNoCredentials so the chain falls through; unknown keys and malformed
// lines produce a hard error.
type FileProvider struct {
	Path    string
	Profile string
	GetEnv  func(string) string
}

// NewFileProvider returns a FileProvider configured with the given profile.
// Path is left to be resolved at Retrieve time so the env override is
// honored.
func NewFileProvider(profile string) *FileProvider {
	return &FileProvider{Profile: profile, GetEnv: os.Getenv}
}

// Retrieve reads and parses the resolved file. When the file or profile
// does not exist, returns ErrNoCredentials. Permissions warnings are emitted
// via log/slog at WARN level.
func (p *FileProvider) Retrieve(_ context.Context) (Credentials, error) {
	if p.GetEnv == nil {
		p.GetEnv = os.Getenv
	}
	path, err := p.resolvePath()
	if err != nil {
		return Credentials{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, errors.Join(ErrNoCredentials, fmt.Errorf("credentials file %s not found", path))
		}
		return Credentials{}, fmt.Errorf("stat credentials file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Credentials{}, fmt.Errorf("credentials file %s is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		slog.Warn("proxmox credentials file is readable by other users; consider chmod 600 or 400",
			"path", path, "mode", info.Mode().Perm().String())
	}
	parsed, err := parseCredentialsFile(path, p.Profile)
	if err != nil {
		return Credentials{}, err
	}
	parsed.Source = p.Name()
	if !parsed.Complete() {
		return Credentials{}, errors.Join(ErrNoCredentials, fmt.Errorf("profile %q in %s is incomplete", p.Profile, path))
	}
	return parsed, nil
}

// Name returns the credentials-file source label.
func (p *FileProvider) Name() string { return "credentials file" }

// resolvePath picks the first non-empty source among Path, the env override,
// and the default ~/.proxmox/credentials.
func (p *FileProvider) resolvePath() (string, error) {
	if p.Path != "" {
		return p.Path, nil
	}
	if env := p.GetEnv(EnvCredentialsFile); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for credentials file: %w", err)
	}
	return filepath.Join(home, ".proxmox", "credentials"), nil
}

// parseCredentialsFile reads path and returns the credentials for the named
// profile. The parser is intentionally minimal: [profile] opens a section,
// "key = value" and "key=value" assign fields, and "#" or ";" start a
// comment to end of line. Unknown keys and empty section names produce
// hard errors so the user is not silently misconfigured.
func parseCredentialsFile(path, profile string) (Credentials, error) {
	f, err := os.Open(path)
	if err != nil {
		return Credentials{}, err
	}
	defer f.Close()

	var (
		current  string
		profiles = map[string]Credentials{}
		lineNum  int
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ";") {
			continue
		}
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			current = strings.TrimSpace(trim[1 : len(trim)-1])
			if current == "" {
				return Credentials{}, fmt.Errorf("%s:%d: empty section header", path, lineNum)
			}
			if _, exists := profiles[current]; exists {
				return Credentials{}, fmt.Errorf("%s:%d: duplicate profile %q", path, lineNum, current)
			}
			profiles[current] = Credentials{}
			continue
		}
		if current == "" {
			return Credentials{}, fmt.Errorf("%s:%d: key=value before any [profile] header", path, lineNum)
		}
		eq := strings.IndexByte(trim, '=')
		if eq < 0 {
			return Credentials{}, fmt.Errorf("%s:%d: expected key=value, got %q", path, lineNum, trim)
		}
		key := strings.TrimSpace(trim[:eq])
		val := strings.TrimSpace(trim[eq+1:])
		c := profiles[current]
		switch key {
		case "api_token":
			c.Token = val
		case "username":
			c.Username = val
		case "password":
			c.Password = val
		case "endpoint":
			c.Endpoint = val
		case "insecure":
			c.Insecure = val
		case "root_ca":
			c.RootCA = val
		case "otp":
			c.OTP = val
		default:
			return Credentials{}, fmt.Errorf("%s:%d: unknown key %q in profile %q", path, lineNum, key, current)
		}
		profiles[current] = c
	}
	if err := scanner.Err(); err != nil {
		return Credentials{}, fmt.Errorf("read credentials file %s: %w", path, err)
	}
	if profile == "" {
		profile = "default"
	}
	c, ok := profiles[profile]
	if !ok {
		return Credentials{}, errors.Join(ErrNoCredentials, fmt.Errorf("profile %q not found in %s", profile, path))
	}
	return c, nil
}
