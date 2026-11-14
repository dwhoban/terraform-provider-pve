// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestGeneratedRegistryValid loads the embedded registry and proves every
// type instantiates its engine and produces a schema. This is the
// compile-time-equivalent guard for generated data: malformed descriptors
// fail here, not in a live cluster.
func TestGeneratedRegistryValid(t *testing.T) {
	reg, err := GeneratedRegistry()
	if err != nil {
		t.Fatalf("GeneratedRegistry: %v", err)
	}
	if len(reg.Types) == 0 {
		t.Fatal("registry is empty — run `go run ./gen`")
	}

	ctx := context.Background()
	seen := map[Kind]map[string]bool{}
	for _, d := range reg.Types {
		if seen[d.Kind] == nil {
			seen[d.Kind] = map[string]bool{}
		}
		if seen[d.Kind][d.Name] {
			t.Errorf("duplicate %s type name %s", d.Kind, d.Name)
		}
		seen[d.Kind][d.Name] = true
		if d.Phase < 1 || d.Phase > 13 {
			t.Errorf("type %s: phase %d out of range", d.Name, d.Phase)
		}
		if d.Summary == "" {
			t.Errorf("type %s: missing summary", d.Name)
		}
		if len(d.Attributes) == 0 && d.Kind == KindDataSource {
			t.Errorf("type %s: no attributes", d.Name)
		}

		switch d.Kind {
		case KindResource:
			r, ok := NewGenericResource(d)().(interface {
				Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)
			})
			if !ok {
				t.Fatalf("type %s: not a resource.Schema provider", d.Name)
			}
			resp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, resp)
			assertNoDiags(t, d.Name, resp.Diagnostics)
		case KindDataSource:
			ds, ok := NewGenericDataSource(d)().(interface {
				Schema(context.Context, datasource.SchemaRequest, *datasource.SchemaResponse)
			})
			if !ok {
				t.Fatalf("type %s: not a datasource.Schema provider", d.Name)
			}
			resp := &datasource.SchemaResponse{}
			ds.Schema(ctx, datasource.SchemaRequest{}, resp)
			assertNoDiags(t, d.Name, resp.Diagnostics)
		case KindAction:
			a, ok := NewGenericAction(d)().(interface {
				Schema(context.Context, action.SchemaRequest, *action.SchemaResponse)
			})
			if !ok {
				t.Fatalf("type %s: not an action.Schema provider", d.Name)
			}
			resp := &action.SchemaResponse{}
			a.Schema(ctx, action.SchemaRequest{}, resp)
			assertNoDiags(t, d.Name, resp.Diagnostics)
		case KindFunction:
			f, ok := NewGenericFunction(d)().(interface {
				Definition(context.Context, function.DefinitionRequest, *function.DefinitionResponse)
			})
			if !ok {
				t.Fatalf("type %s: not a function.Definition provider", d.Name)
			}
			resp := &function.DefinitionResponse{}
			f.Definition(ctx, function.DefinitionRequest{}, resp)
			assertNoDiags(t, d.Name, resp.Diagnostics)
		default:
			t.Errorf("type %s: unknown kind %q", d.Name, d.Kind)
		}
	}

	for _, kind := range []Kind{KindResource, KindDataSource, KindAction, KindFunction} {
		if len(seen[kind]) == 0 {
			t.Errorf("registry contains no %s types", kind)
		}
	}
}

func assertNoDiags(t *testing.T, name string, diags interface{ HasError() bool }) {
	t.Helper()
	if diags.HasError() {
		t.Errorf("type %s: schema diagnostics: %v", name, diags)
	}
}

// TestRegistryRegistrationWiring proves the provider registers exactly the
// registry's types under each interface.
func TestRegistryRegistrationWiring(t *testing.T) {
	reg, err := GeneratedRegistry()
	if err != nil {
		t.Fatalf("GeneratedRegistry: %v", err)
	}
	p := &pveProvider{}
	counts := map[Kind]int{}
	for _, d := range reg.Types {
		counts[d.Kind]++
	}
	if got := len(p.Resources(context.Background())); got != counts[KindResource]+1 { // + pve_cluster_qdevice (hand-written, needs ssh)
		t.Errorf("Resources() = %d, want %d", got, counts[KindResource]+1)
	}
	if got := len(p.DataSources(context.Background())); got != counts[KindDataSource]+1 { // + pve_version
		t.Errorf("DataSources() = %d, want %d", got, counts[KindDataSource]+1)
	}
	if got := len(p.Actions(context.Background())); got != counts[KindAction]+1 { // + pve_ceph_install (hand-written, needs ssh)
		t.Errorf("Actions() = %d, want %d", got, counts[KindAction]+1)
	}
	if got := len(p.Functions(context.Background())); got != counts[KindFunction] {
		t.Errorf("Functions() = %d, want %d", got, counts[KindFunction])
	}
	// Every constructor must yield a working instance.
	for _, mk := range p.Resources(context.Background()) {
		if r := mk(); r == nil {
			t.Fatal("nil resource constructor")
		}
	}
	for _, mk := range p.Actions(context.Background()) {
		if a := mk(); a == nil {
			t.Fatal("nil action constructor")
		}
	}
	for _, mk := range p.Functions(context.Background()) {
		if f := mk(); f == nil {
			t.Fatal("nil function constructor")
		}
	}
	_ = fmt.Sprint()
}
