// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAttrCodec(t *testing.T) {
	str := Attr{Name: "migration", Type: AttrString}
	migration := map[string]any{"type": "secure", "network": "10.0.100.0/24"}

	t.Run("decode object as property string", func(t *testing.T) {
		v, err := decodeAttr(str, migration)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := v.(types.String); !ok || got.ValueString() != "network=10.0.100.0/24,type=secure" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("decode mixed value kinds", func(t *testing.T) {
		v, err := decodeAttr(str, map[string]any{"b": true, "n": json.Number("3"), "z": nil, "l": []any{"x"}})
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := v.(types.String); !ok || got.ValueString() != `b=1,l=["x"],n=3` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("encode base64", func(t *testing.T) {
		got, err := encodeAttr(Attr{Name: "data", Type: AttrString, Format: FormatBase64}, types.StringValue("A=1\nB=2"))
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"QT0xCkI9Mg=="}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	for _, tc := range []struct {
		cur  string
		want bool
	}{
		{"type=secure,network=10.0.100.0/24", true},
		{"network=10.0.100.0/24,type=secure", true},
		{"type=insecure,network=10.0.100.0/24", false},
		{"type=secure", false},
		{"type=secure,network=10.0.100.0/24,extra=1", false},
		{"type=secure,type=secure", false},
		{"garbage", false},
	} {
		t.Run("wireEqual "+tc.cur, func(t *testing.T) {
			if got := wireEqual(str, types.StringValue(tc.cur), migration); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if wireEqual(str, types.StringNull(), migration) {
		t.Fatal("null current value must not be wire-equal")
	}
	if wireEqual(str, types.StringValue("secure"), "secure") {
		t.Fatal("plain values compare through decode, not wireEqual")
	}

	for payload, want := range map[string]string{
		`"https://x"`: "https://x",
		`{"a":1}`:     `{"a":1}`,
	} {
		if got := decodeResultString(json.RawMessage(payload)); got != want {
			t.Fatalf("decodeResultString(%s) = %q, want %q", payload, got, want)
		}
	}
}

// guestBlockSpecs mirrors the generator's pve_vm disk/network block
// declarations (gen/mapping/phase3.go) for codec-level tests.
func guestBlockSpecs() (disk, network Attr) {
	disk = Attr{Name: "disk", Type: AttrList, Optional: true, ElemAttrs: []Attr{
		{Name: "interface", Type: AttrString, Required: true},
		{Name: "storage", Type: AttrString, Optional: true},
		{Name: "size_gb", Type: AttrInt64, Optional: true, Computed: true},
		{Name: "volume_id", Type: AttrString, Optional: true, Computed: true},
		{Name: "iothread", Type: AttrBool, Optional: true},
		{Name: "discard", Type: AttrBool, Optional: true},
	}, Block: &BlockSpec{
		Families:   []string{"ide", "sata", "scsi", "virtio"},
		DefaultKey: "file",
		Exclude:    map[string]string{"media": "cdrom", "file": "*cloudinit*"},
		Fields: []BlockField{
			{Name: "interface", Key: "interface", Role: RoleIndex},
			{Name: "storage", Key: "file", Role: RoleVolumeStorage, CreateOnly: true},
			{Name: "size_gb", Key: "size", Role: RoleVolumeSizeGB, CreateOnly: true},
			{Name: "volume_id", Key: "file", Role: RoleVolumeID, CreateOnly: true},
			{Name: "iothread", Key: "iothread"},
			{Name: "discard", Key: "discard", True: "on", False: "ignore"},
		},
	}}
	network = Attr{Name: "network", Type: AttrList, Optional: true, ElemAttrs: []Attr{
		{Name: "model", Type: AttrString, Optional: true, Computed: true, Enum: []string{"virtio", "e1000"}},
		{Name: "mac_address", Type: AttrString, Optional: true, Computed: true},
		{Name: "bridge", Type: AttrString, Optional: true},
		{Name: "vlan", Type: AttrInt64, Optional: true},
		{Name: "ipv4", Type: AttrString, Optional: true},
	}, Block: &BlockSpec{
		Families:   []string{"net"},
		DefaultKey: "model",
		Fields: []BlockField{
			{Name: "model", Key: "model", Role: RoleModel},
			{Name: "mac_address", Key: "macaddr", Role: RoleMAC},
			{Name: "bridge", Key: "bridge"},
			{Name: "vlan", Key: "tag"},
			{Name: "ipv4", Family: "ipconfig", Key: "ip"},
		},
	}}
	return disk, network
}

func TestBlockCodec(t *testing.T) {
	disk, network := guestBlockSpecs()

	t.Run("encode disk", func(t *testing.T) {
		v, err := encodeBlock(disk, blockValue(disk, []map[string]attr.Value{{
			"interface": types.StringValue("scsi0"),
			"storage":   types.StringValue("local-lvm"),
			"size_gb":   types.Int64Value(32),
			"iothread":  types.BoolValue(true),
			"discard":   types.BoolValue(true),
		}}))
		if err != nil {
			t.Fatal(err)
		}
		if got := v.Get("scsi0"); got != "local-lvm:32,iothread=1,discard=on" {
			t.Fatalf("scsi0 = %q", got)
		}
	})
	t.Run("encode network position and secondary", func(t *testing.T) {
		v, err := encodeBlock(network, blockValue(network, []map[string]attr.Value{{
			"model":  types.StringValue("virtio"),
			"bridge": types.StringValue("vmbr1"),
			"vlan":   types.Int64Value(30),
			"ipv4":   types.StringValue("dhcp"),
		}}))
		if err != nil {
			t.Fatal(err)
		}
		if got := v.Get("net0"); got != "virtio,bridge=vmbr1,tag=30" {
			t.Fatalf("net0 = %q", got)
		}
		if got := v.Get("ipconfig0"); got != "ip=dhcp" {
			t.Fatalf("ipconfig0 = %q", got)
		}
	})
	t.Run("duplicate slot", func(t *testing.T) {
		elem := func(storage string) map[string]attr.Value {
			return map[string]attr.Value{
				"interface": types.StringValue("scsi0"),
				"storage":   types.StringValue(storage),
				"size_gb":   types.Int64Value(1),
			}
		}
		_, err := encodeBlock(disk, blockValue(disk, []map[string]attr.Value{elem("local-lvm"), elem("other")}))
		if err == nil || !strings.Contains(err.Error(), "used by two blocks") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("decode disk filters cdrom and cloudinit", func(t *testing.T) {
		elems, present, err := decodeBlockSlots(disk, map[string]any{
			"scsi0": "local-lvm:vm-120-disk-0,iothread=1,discard=on,size=32G",
			"ide2":  "local:iso/d.iso,media=cdrom",
			"ide0":  "local-lvm:vm-120-cloudinit,media=cdrom",
		})
		if err != nil || !present {
			t.Fatalf("elems=%v present=%v err=%v", elems, present, err)
		}
		if len(elems) != 1 || elems[0].key != "scsi0" {
			t.Fatalf("elems = %+v", elems)
		}
		f := elems[0].fields
		if got := blockStr(t, f, "volume_id"); got != "local-lvm:vm-120-disk-0" {
			t.Fatalf("volume_id = %q", got)
		}
		if got := blockStr(t, f, "storage"); got != "local-lvm" {
			t.Fatalf("storage = %q", got)
		}
		if got := blockInt(t, f, "size_gb"); got != 32 {
			t.Fatalf("size_gb = %d", got)
		}
		if !blockBool(t, f, "iothread") || !blockBool(t, f, "discard") {
			t.Fatalf("flags = %v %v", f["iothread"], f["discard"])
		}
	})
	t.Run("decode model=mac", func(t *testing.T) {
		elems, present, err := decodeBlockSlots(network, map[string]any{
			"net0": "virtio=BC:24:11:00:00:01,bridge=vmbr0",
		})
		if err != nil || !present {
			t.Fatalf("elems=%v present=%v err=%v", elems, present, err)
		}
		f := elems[0].fields
		if got := blockStr(t, f, "model"); got != "virtio" {
			t.Fatalf("model = %q", got)
		}
		if got := blockStr(t, f, "mac_address"); got != "BC:24:11:00:00:01" {
			t.Fatalf("mac_address = %q", got)
		}
	})
	t.Run("position gap errors", func(t *testing.T) {
		_, _, err := decodeBlockSlots(network, map[string]any{"net0": "virtio", "net2": "virtio"})
		if err == nil || !strings.Contains(err.Error(), "no net1") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("size rounding", func(t *testing.T) {
		gb, err := parseSizeGB("2252M")
		if err != nil {
			t.Fatal(err)
		}
		if gb != 3 {
			t.Fatalf("2252M = %d GiB", gb)
		}
	})
}

func blockStr(t *testing.T, f map[string]attr.Value, name string) string {
	t.Helper()
	v, ok := f[name].(types.String)
	if !ok {
		t.Fatalf("%s is %T, want string", name, f[name])
	}
	return v.ValueString()
}

func blockInt(t *testing.T, f map[string]attr.Value, name string) int64 {
	t.Helper()
	v, ok := f[name].(types.Int64)
	if !ok {
		t.Fatalf("%s is %T, want int64", name, f[name])
	}
	return v.ValueInt64()
}

func blockBool(t *testing.T, f map[string]attr.Value, name string) bool {
	t.Helper()
	v, ok := f[name].(types.Bool)
	if !ok {
		t.Fatalf("%s is %T, want bool", name, f[name])
	}
	return v.ValueBool()
}
