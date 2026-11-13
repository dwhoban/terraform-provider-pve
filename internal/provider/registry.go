// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// The provider type registry is generated data: gen/ (run via
// `go generate`) transforms the parsed Proxmox VE API schema plus the
// ROADMAP.md mapping into registry.json, embedded here and instantiated by
// the generic engines.

//go:embed registry.json
var registryJSON []byte

var (
	registryOnce sync.Once
	registryVal  *Registry
	registryErr  error
)

// GeneratedRegistry returns the embedded registry, decoding it once.
func GeneratedRegistry() (*Registry, error) {
	registryOnce.Do(func() {
		registryVal = &Registry{}
		if err := json.Unmarshal(registryJSON, registryVal); err != nil {
			registryErr = fmt.Errorf("decoding embedded registry.json: %w", err)
		}
	})
	return registryVal, registryErr
}

// registryConstructors maps registry entries of the matching kind to
// constructor functions.
func registryConstructors[T any](kind Kind, mk func(*TypeDescriptor) func() T) []func() T {
	reg, err := GeneratedRegistry()
	if err != nil {
		panic(err) // embedded data is validated by tests; unreachable in practice
	}
	var out []func() T
	for _, d := range reg.Types {
		if d.Kind != kind {
			continue
		}
		out = append(out, mk(d))
	}
	return out
}
