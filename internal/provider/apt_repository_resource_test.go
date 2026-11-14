// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const aptEnterpriseFile = "/etc/apt/sources.list.d/pve-enterprise.sources"

func aptRepoModel(filePath string, index int64, enabled types.Bool) *aptRepositoryModel {
	return &aptRepositoryModel{
		Node: types.StringValue("pve1"), FilePath: types.StringValue(filePath),
		Index: types.Int64Value(index), Enabled: enabled,
	}
}

func aptRepoTestResource(t *testing.T) (*aptRepositoryResource, *aptFake) {
	t.Helper()
	std, fake := aptTestResource(t)
	return &aptRepositoryResource{client: std.client}, fake
}

func TestAptRepositoryDisable(t *testing.T) {
	r, fake := aptRepoTestResource(t)
	m := aptRepoModel(aptEnterpriseFile, 0, types.BoolValue(false))
	if diags := r.converge(context.Background(), m); diags.HasError() {
		t.Fatal(diags)
	}
	want := []string{"POST path=" + aptEnterpriseFile + " index=0 enabled=0"}
	if !reflect.DeepEqual(fake.mutations, want) {
		t.Errorf("mutations = %v, want %v", fake.mutations, want)
	}
	var uris []string
	m.URIs.ElementsAs(context.Background(), &uris, false)
	if !reflect.DeepEqual(uris, []string{"https://enterprise.proxmox.com/debian/pve"}) {
		t.Errorf("uris = %v", uris)
	}
	if m.Enabled.ValueBool() {
		t.Error("model still enabled")
	}
}

func TestAptRepositoryEnableNonStandard(t *testing.T) {
	r, fake := aptRepoTestResource(t)
	const p = "/etc/apt/sources.list.d/custom.sources"
	fake.files[p] = []fakeAptEntry{{"http://repo.example/debian", "main", false}}
	fake.fileOrder = append(fake.fileOrder, p)

	m := aptRepoModel(p, 0, types.BoolValue(true))
	if diags := r.converge(context.Background(), m); diags.HasError() {
		t.Fatal(diags)
	}
	want := []string{"POST path=" + p + " index=0 enabled=1"}
	if !reflect.DeepEqual(fake.mutations, want) {
		t.Errorf("mutations = %v, want %v", fake.mutations, want)
	}
	if !m.Enabled.ValueBool() {
		t.Error("model not enabled")
	}
}

func TestAptRepositoryAdoptWhenEnabledUnset(t *testing.T) {
	r, fake := aptRepoTestResource(t)
	m := aptRepoModel(aptEnterpriseFile, 0, types.BoolUnknown())
	if diags := r.converge(context.Background(), m); diags.HasError() {
		t.Fatal(diags)
	}
	if len(fake.mutations) != 0 {
		t.Errorf("mutations = %v, want none", fake.mutations)
	}
	if !m.Enabled.ValueBool() {
		t.Error("enabled not adopted")
	}
}

func TestAptRepositoryMissingEntry(t *testing.T) {
	r, fake := aptRepoTestResource(t)
	diags := r.converge(context.Background(), aptRepoModel(aptEnterpriseFile, 5, types.BoolValue(false)))
	if !diags.HasError() || diags.Errors()[0].Summary() != "Repository Entry Not Found" {
		t.Fatalf("diags = %v", diags)
	}
	if len(fake.mutations) != 0 {
		t.Errorf("mutations = %v, want none", fake.mutations)
	}
}

func TestAptRepositoryImportID(t *testing.T) {
	for _, c := range []struct {
		id    string
		ok    bool
		path  string
		index int64
	}{
		{"pve1,/etc/apt/sources.list,0", true, "/etc/apt/sources.list", 0},
		{"pve1,/a,b/c.sources,2", true, "/a,b/c.sources", 2},
		{"pve1,/x", false, "", 0},
		{"pve1,etc/x,0", false, "", 0},
		{"pve1,/x,-1", false, "", 0},
	} {
		node, p, i, ok := parseAptRepositoryImportID(c.id)
		if ok != c.ok || p != c.path || i != c.index || (ok && node != "pve1") {
			t.Errorf("%q: got (%q, %q, %d, %v)", c.id, node, p, i, ok)
		}
	}
}
