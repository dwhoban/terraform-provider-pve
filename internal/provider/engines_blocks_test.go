// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	pathpkg "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// fakeGuestPVE is a QEMU-config-shaped API stub for the slot-family block
// engine tests: POST/PUT store the request form verbatim, GET answers the
// stored config with PVE-style rewrites (allocated volumes carry
// vm-<id>-disk-<n> and size=<N>G; NICs come back as <model>=<MAC>), and
// the canonical `delete` parameter unsets keys.
type fakeGuestPVE struct {
	mu    sync.Mutex
	cfg   map[string]string
	sizes map[string]string // slot → allocated size, echoed on every read
}

var guestSlotRe = regexp.MustCompile(`^(ide|sata|scsi|virtio|net|mp|unused)(\d+)$`)

func newFakeGuestPVE() *fakeGuestPVE {
	return &fakeGuestPVE{cfg: map[string]string{}, sizes: map[string]string{}}
}

func (f *fakeGuestPVE) rewrite(key, v string) string {
	idxS := regexp.MustCompile(`\d+$`).FindString(key)
	fam := strings.TrimSuffix(key, idxS)
	parts := strings.Split(v, ",")
	head := parts[0]
	switch fam {
	case "ide", "sata", "scsi", "virtio":
		storage, size, found := strings.Cut(head, ":")
		// Only an allocation (storage:<number>) is rewritten; a concrete
		// volume id stays as stored. PVE echoes the allocated size on
		// every read, whichever form was written.
		if found && strings.Trim(size, "0123456789") == "" {
			f.sizes[key] = size
			head = fmt.Sprintf("%s:vm-101-disk-%s", storage, idxS)
		}
		if s, ok := f.sizes[key]; ok {
			head = fmt.Sprintf("%s,size=%sG", head, s)
		}
	case "net":
		if model, _, found := strings.Cut(head, "="); !found {
			head = fmt.Sprintf("%s=BC:24:11:00:00:0%s", model, idxS)
		}
	}
	parts[0] = head
	out := parts[0]
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "size=") {
			out += "," + p
		}
	}
	return out
}

func (f *fakeGuestPVE) client(t *testing.T) *pveclient.Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL: srv.URL, Credentials: pveclient.Credentials{APIToken: "root@pam!t=test"},
		Insecure: true, RetryMax: 1, RetryBaseWait: 1, RequestTimeout: 0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func (f *fakeGuestPVE) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	switch {
	case regexp.MustCompile(`^/nodes/[^/]+/qemu$`).MatchString(path) && r.Method == http.MethodPost:
		_ = r.ParseForm()
		f.mu.Lock()
		for k, vs := range r.PostForm {
			if k == "delete" {
				for _, d := range strings.Split(vs[0], ",") {
					delete(f.cfg, d)
				}
				continue
			}
			f.cfg[k] = vs[0]
		}
		f.mu.Unlock()
		writeData(w, nil)
	case regexp.MustCompile(`^/nodes/[^/]+/qemu/\d+/config$`).MatchString(path):
		switch r.Method {
		case http.MethodGet:
			f.mu.Lock()
			defer f.mu.Unlock()
			out := map[string]any{"name": f.cfg["name"]}
			for k, v := range f.cfg {
				if m := guestSlotRe.FindStringSubmatch(k); m != nil {
					out[k] = f.rewrite(k, v)
					continue
				}
				if k == "name" || k == "delete" {
					continue
				}
				out[k] = v
			}
			writeData(w, out)
		case http.MethodPut:
			_ = r.ParseForm()
			f.mu.Lock()
			for k, vs := range r.PostForm {
				if k == "delete" {
					for _, d := range strings.Split(vs[0], ",") {
						delete(f.cfg, d)
					}
					continue
				}
				f.cfg[k] = vs[0]
			}
			f.mu.Unlock()
			writeData(w, nil)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		writeErr(w, http.StatusInternalServerError, "unhandled "+r.Method+" "+path)
	}
}

func guestDescriptor(t *testing.T) *TypeDescriptor {
	disk, network := registryAttr(t, "pve_vm", "disk"), registryAttr(t, "pve_vm", "network")
	return &TypeDescriptor{
		Name: "pve_guest", Kind: KindResource, Phase: 3,
		IDAttrs: []string{"node", "vmid"},
		Create:  &Op{Method: "POST", Path: "/nodes/{node}/qemu", Params: []string{"name", "disk", "network"}},
		Read:    &Op{Method: "GET", Path: "/nodes/{node}/qemu/{vmid}/config"},
		Update:  &Op{Method: "PUT", Path: "/nodes/{node}/qemu/{vmid}/config", Params: []string{"name", "disk", "network"}, DeleteParam: true},
		Delete:  &Op{Method: "DELETE", Path: "/nodes/{node}/qemu/{vmid}"},
		Attributes: []Attr{
			{Name: "node", Type: AttrString, Required: true, ForceNew: true, PathParam: "node"},
			{Name: "vmid", Type: AttrInt64, Required: true, ForceNew: true, PathParam: "vmid"},
			{Name: "name", Type: AttrString, Required: true},
			disk,
			network,
		},
	}
}

func blockElemValue(a Attr, fields map[string]attr.Value) attr.Value {
	elems := []map[string]attr.Value{fields}
	return blockValue(a, elems)
}

func TestEngineGuestBlocksRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newFakeGuestPVE()
	d := guestDescriptor(t)
	r := &genericResource{engineBase: engineBase{desc: d, client: f.client(t)}}
	schema := buildResourceSchema(d)

	diskAttr, _ := d.Attr("disk")
	netAttr, _ := d.Attr("network")

	plan := func(networks attr.Value) map[string]attr.Value {
		return map[string]attr.Value{
			"node": types.StringValue("node1"),
			"vmid": types.Int64Value(101),
			"name": types.StringValue("web"),
			"disk": blockElemValue(diskAttr, map[string]attr.Value{
				"interface": types.StringValue("scsi0"),
				"storage":   types.StringValue("local-lvm"),
				"size_gb":   types.Int64Value(32),
			}),
			"network": networks,
		}
	}
	net := func(bridge, ipv4 string) map[string]attr.Value {
		return map[string]attr.Value{
			"model":  types.StringValue("virtio"),
			"bridge": types.StringValue(bridge),
			"ipv4":   types.StringValue(ipv4),
		}
	}
	twoNICs := blockValue(netAttr, []map[string]attr.Value{net("vmbr0", "10.0.0.20/24"), net("vmbr1", "dhcp")})
	oneNIC := blockValue(netAttr, []map[string]attr.Value{net("vmbr0", "10.0.0.20/24")})

	// Create: one disk, two NICs.
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTF(d, plan(twoNICs)), Schema: schema},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics)
	}
	var disks types.List
	if diags := createResp.State.GetAttribute(ctx, pathpkg.Root("disk"), &disks); diags.HasError() {
		t.Fatalf("state disk: %v", diags)
	}
	d0 := blockElemAt(t, disks, 0)
	if got := blockStr(t, d0, "volume_id"); got != "local-lvm:vm-101-disk-0" {
		t.Fatalf("volume_id = %q", got)
	}
	if got := blockInt(t, d0, "size_gb"); got != 32 {
		t.Fatalf("size_gb = %d", got)
	}
	if got := blockStr(t, d0, "storage"); got != "local-lvm" {
		t.Fatalf("storage = %q", got)
	}
	var nics types.List
	if diags := createResp.State.GetAttribute(ctx, pathpkg.Root("network"), &nics); diags.HasError() {
		t.Fatalf("state network: %v", diags)
	}
	if len(nics.Elements()) != 2 {
		t.Fatalf("state network len = %d", len(nics.Elements()))
	}
	n0 := blockElemAt(t, nics, 0)
	if got := blockStr(t, n0, "mac_address"); got != "BC:24:11:00:00:00" {
		t.Fatalf("mac_address = %q", got)
	}
	if got := blockStr(t, n0, "bridge"); got != "vmbr0" {
		t.Fatalf("bridge = %q", got)
	}
	if _, sent := f.cfg["ipconfig1"]; !sent || f.cfg["ipconfig1"] != "ip=dhcp" {
		t.Fatalf("ipconfig1 = %q", f.cfg["ipconfig1"])
	}

	// Update removing network[1]: net1 and ipconfig1 are deleted.
	updPlan := plan(oneNIC)
	updPlan["disk"] = blockElemValue(diskAttr, map[string]attr.Value{
		"interface": types.StringValue("scsi0"),
		"storage":   types.StringValue("local-lvm"),
		"size_gb":   types.Int64Value(32),
		"volume_id": types.StringValue("local-lvm:vm-101-disk-0"),
	})
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Raw: toTF(d, updPlan), Schema: schema},
		State: tfsdk.State{Raw: createResp.State.Raw, Schema: schema},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics)
	}
	if _, still := f.cfg["net1"]; still {
		t.Fatal("net1 not deleted")
	}
	if _, still := f.cfg["ipconfig1"]; still {
		t.Fatal("ipconfig1 not deleted")
	}

	// Resizing an existing disk is rejected with a pointer to the action.
	grow := plan(oneNIC)
	grow["disk"] = blockElemValue(diskAttr, map[string]attr.Value{
		"interface": types.StringValue("scsi0"),
		"storage":   types.StringValue("local-lvm"),
		"size_gb":   types.Int64Value(40),
		"volume_id": types.StringValue("local-lvm:vm-101-disk-0"),
	})
	growResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Raw: toTF(d, grow), Schema: schema},
		State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema},
	}, growResp)
	if !growResp.Diagnostics.HasError() {
		t.Fatal("growing size_gb should fail")
	}
	if detail := growResp.Diagnostics[0].Detail(); !strings.Contains(detail, "size_gb cannot change on an existing disk") {
		t.Fatalf("detail = %s", detail)
	}

	// Import: identity-only state adopts the server's slots.
	f.cfg["net1"] = "virtio,bridge=vmbr1"
	f.cfg["ipconfig1"] = "ip=dhcp"
	importValues := map[string]attr.Value{"id": types.StringValue("node1/101")}
	readResp := &resource.ReadResponse{State: tfsdk.State{Raw: toTF(d, importValues), Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: toTF(d, importValues), Schema: schema}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("import Read: %v", readResp.Diagnostics)
	}
	if diags := readResp.State.GetAttribute(ctx, pathpkg.Root("network"), &nics); diags.HasError() {
		t.Fatalf("import network: %v", diags)
	}
	if len(nics.Elements()) != 2 {
		t.Fatalf("imported network len = %d", len(nics.Elements()))
	}
	n1 := blockElemAt(t, nics, 1)
	if got := blockStr(t, n1, "ipv4"); got != "dhcp" {
		t.Fatalf("imported ipv4[1] = %q", got)
	}
}

// blockElemAt returns element i of a block list as its attribute map.
func blockElemAt(t *testing.T, list types.List, i int) map[string]attr.Value {
	t.Helper()
	e, ok := list.Elements()[i].(types.Object)
	if !ok {
		t.Fatalf("element %d is %T, want object", i, list.Elements()[i])
	}
	return e.Attributes()
}
