// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestClient_GetCephStatus_Decodes covers the node-scoped raw status GET and
// the typed summary fields derived from it.
func TestClient_GetCephStatus_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/ceph/status" {
			_, _ = io.WriteString(w, `{"data":{"health":{"status":"HEALTH_WARN"},"mon":{"quorum":[0,1],"quorum_names":["pve1","pve2"]},"version":{"19.2.0":3}}}`)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	status, err := c.GetCephStatus(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetCephStatus: %v", err)
	}
	if status.Health != "HEALTH_WARN" {
		t.Fatalf("Health = %q", status.Health)
	}
	if len(status.QuorumNames) != 2 || status.QuorumNames[1] != "pve2" {
		t.Fatalf("QuorumNames = %v", status.QuorumNames)
	}
	if len(status.Quorum) != 2 || status.Quorum[1] != 1 {
		t.Fatalf("Quorum = %v", status.Quorum)
	}
	if status.Versions["19.2.0"] != 3 {
		t.Fatalf("Versions = %v", status.Versions)
	}
	if !strings.Contains(status.RawJSON, `"HEALTH_WARN"`) {
		t.Fatalf("RawJSON = %q", status.RawJSON)
	}
}

// TestClient_GetCephMetadata_Decodes covers the cluster-scoped metadata GET.
func TestClient_GetCephMetadata_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/cluster/ceph/metadata" {
			_, _ = io.WriteString(w, `{"data":{"mon":{"pve1":{"id":0}}}}`)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	raw, err := c.GetCephMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetCephMetadata: %v", err)
	}
	if !strings.Contains(raw, `"pve1"`) {
		t.Fatalf("metadata = %q", raw)
	}
}

// TestClient_CephPool_CRUD walks list/create/update/delete plus the
// found and not-found single-pool reads.
func TestClient_CephPool_CRUD(t *testing.T) {
	listing := `{"data":[{"pool":2,"pool_name":"pool1","size":3,"min_size":2,"pg_num":128,` +
		`"pg_num_min":32,"pg_num_final":128,"pg_autoscale_mode":"warn","crush_rule":1,` +
		`"crush_rule_name":"replicated_rule","target_size_ratio":0.3,"type":"replicated",` +
		`"bytes_used":1024,"percent_used":0.5,"application_metadata":{"rbd":{}},"autoscale_status":{"mode":"warn"}}]}`
	var createBody, updateBody map[string]any
	var deleteQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/ceph/pool":
			_, _ = io.WriteString(w, listing)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/pool":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &createBody); err != nil {
				t.Fatalf("create body not JSON: %v", err)
			}
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephcreatepool:root@pam:"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/nodes/pve1/ceph/pool/pool1":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &updateBody); err != nil {
				t.Fatalf("update body not JSON: %v", err)
			}
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephsetpool:root@pam:"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/ceph/pool/pool1":
			deleteQuery = r.URL.RawQuery
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephdestroypool:root@pam:"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	pools, err := c.ListCephPools(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListCephPools: %v", err)
	}
	if len(pools) != 1 || pools[0].PoolName != "pool1" || pools[0].PGNum != 128 {
		t.Fatalf("pools = %+v", pools)
	}
	if pools[0].PGNumMin == nil || *pools[0].PGNumMin != 32 {
		t.Fatalf("PGNumMin = %v", pools[0].PGNumMin)
	}
	if !strings.Contains(string(pools[0].ApplicationMetadata), "rbd") {
		t.Fatalf("ApplicationMetadata = %s", pools[0].ApplicationMetadata)
	}

	create, err := c.CreateCephPool(context.Background(), "pve1", CreateCephPoolInput{
		Name:            "pool1",
		Size:            intPtr(3),
		MinSize:         intPtr(2),
		PGNum:           intPtr(128),
		PGAutoscaleMode: "warn",
		Application:     "rbd",
		CrushRule:       "replicated_rule",
		TargetSizeRatio: cephFloatPtr(0.3),
		AddStorages:     boolPtr(true),
		ErasureCoding:   "k=2,m=1,failure-domain=host",
	})
	if err != nil {
		t.Fatalf("CreateCephPool: %v", err)
	}
	if !strings.Contains(create, "cephcreatepool") {
		t.Fatalf("create upid = %q", create)
	}
	if createBody["name"] != "pool1" || createBody["erasure-coding"] != "k=2,m=1,failure-domain=host" || createBody["add_storages"] != true {
		t.Fatalf("create body = %v", createBody)
	}

	updated, err := c.UpdateCephPool(context.Background(), "pve1", "pool1", UpdateCephPoolInput{
		Size:            intPtr(4),
		PGNum:           intPtr(256),
		PGAutoscaleMode: "on",
	})
	if err != nil {
		t.Fatalf("UpdateCephPool: %v", err)
	}
	if !strings.Contains(updated, "cephsetpool") {
		t.Fatalf("update upid = %q", updated)
	}
	if updateBody["size"] != float64(4) || updateBody["pg_num"] != float64(256) {
		t.Fatalf("update body = %v", updateBody)
	}

	destroyed, err := c.DeleteCephPool(context.Background(), "pve1", "pool1", true, false, true)
	if err != nil {
		t.Fatalf("DeleteCephPool: %v", err)
	}
	if !strings.Contains(destroyed, "cephdestroypool") {
		t.Fatalf("delete upid = %q", destroyed)
	}
	for _, want := range []string{"force=1", "remove_ecprofile=0", "remove_storages=1"} {
		if !strings.Contains(deleteQuery, want) {
			t.Fatalf("delete query = %q, want %q", deleteQuery, want)
		}
	}

	pool, err := c.GetCephPool(context.Background(), "pve1", "pool1")
	if err != nil {
		t.Fatalf("GetCephPool: %v", err)
	}
	if pool == nil || pool.CrushRuleName != "replicated_rule" || pool.TargetSizeRatio == nil || *pool.TargetSizeRatio != 0.3 {
		t.Fatalf("pool = %+v", pool)
	}
	if _, err := c.GetCephPool(context.Background(), "pve1", "missing"); !isTestNotFound(err) {
		t.Fatalf("GetCephPool(missing) err = %v, want 404 APIError", err)
	}
}

// TestClient_CephOSD_CreateDestroy covers the osd create body shape, the
// out/in verbs, and destroy with the cleanup query flag.
func TestClient_CephOSD_CreateDestroy(t *testing.T) {
	var createBody map[string]any
	var destroyQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/osd":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &createBody); err != nil {
				t.Fatalf("create body not JSON: %v", err)
			}
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephcreateosd:root@pam:"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/osd/3/out":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/osd/3/in":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/ceph/osd/3":
			destroyQuery = r.URL.RawQuery
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephdestroyosd:root@pam:"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	upid, err := c.CreateCephOSD(context.Background(), "pve1", CreateCephOSDInput{
		Dev:              "/dev/sdb",
		DBDev:            "/dev/nvme0n1",
		DBDevSizeGiB:     10,
		CrushDeviceClass: "ssd",
		Encrypted:        true,
		OSDsPerDevice:    1,
	})
	if err != nil {
		t.Fatalf("CreateCephOSD: %v", err)
	}
	if !strings.Contains(upid, "cephcreateosd") {
		t.Fatalf("create upid = %q", upid)
	}
	if createBody["dev"] != "/dev/sdb" || createBody["db_dev"] != "/dev/nvme0n1" ||
		createBody["db_dev_size"] != float64(10) || createBody["crush-device-class"] != "ssd" ||
		createBody["encrypted"] != true || createBody["osds-per-device"] != float64(1) {
		t.Fatalf("create body = %v", createBody)
	}

	if err := c.CephOSDOut(context.Background(), "pve1", 3); err != nil {
		t.Fatalf("CephOSDOut: %v", err)
	}
	if err := c.CephOSDIn(context.Background(), "pve1", 3); err != nil {
		t.Fatalf("CephOSDIn: %v", err)
	}

	destroyed, err := c.DestroyCephOSD(context.Background(), "pve1", 3, true)
	if err != nil {
		t.Fatalf("DestroyCephOSD: %v", err)
	}
	if !strings.Contains(destroyed, "cephdestroyosd") {
		t.Fatalf("destroy upid = %q", destroyed)
	}
	if destroyQuery != "cleanup=1" {
		t.Fatalf("destroy query = %q", destroyQuery)
	}
}

// TestClient_CephOSD_Metadata_Decodes covers the single-OSD metadata read,
// including the boolish `encrypted` field.
func TestClient_CephOSD_Metadata_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/ceph/osd/0/metadata" {
			_, _ = io.WriteString(w, `{"data":{"osd":{"id":0,"hostname":"pve1",`+
				`"osd_data":"/var/lib/ceph/osd/pve1-ceph-0","osd_objectstore":"bluestore",`+
				`"encrypted":1,"pid":1234,"version":"19.2.0","front_addr":"[::]:6800",`+
				`"back_addr":"[::]:6801","hb_front_addr":"[::]:6802","hb_back_addr":"[::]:6803",`+
				`"mem_usage":1024},"devices":[{"dev_node":"sdb","device":"block",`+
				`"physical_device":"/dev/sdb","size":1000,"support_discard":true,"type":"hdd"}]}}`)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	meta, err := c.GetCephOSDMetadata(context.Background(), "pve1", 0)
	if err != nil {
		t.Fatalf("GetCephOSDMetadata: %v", err)
	}
	if meta.ID != 0 || meta.Hostname != "pve1" || !meta.Encrypted || meta.PID == nil || *meta.PID != 1234 {
		t.Fatalf("metadata = %+v", meta)
	}
	if len(meta.Devices) != 1 || meta.Devices[0].Device != "block" || !meta.Devices[0].SupportDiscard {
		t.Fatalf("devices = %+v", meta.Devices)
	}
}

// TestClient_ListCephOSDTree_Decodes walks the CRUSH tree listing and
// collects the OSD leaves only.
func TestClient_ListCephOSDTree_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/ceph/osd" {
			_, _ = io.WriteString(w, `{"data":{"flags":"noup","root":{"id":-1,"name":"default",`+
				`"type":"root","children":[{"id":-2,"name":"pve1","type":"host","children":[`+
				`{"id":0,"name":"osd.0","type":"osd","status":"up","class":"ssd"},`+
				`{"id":1,"name":"osd.1","type":"osd","status":"down"}]}]}}}`)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	entries, err := c.ListCephOSDTree(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListCephOSDTree: %v", err)
	}
	if len(entries) != 2 || entries[0].ID != 0 || entries[0].Name != "osd.0" ||
		entries[0].Status != "up" || entries[0].Class != "ssd" || entries[1].Status != "down" {
		t.Fatalf("entries = %+v", entries)
	}
}

// TestClient_CephMon_CreateDestroyList covers mon create (with and without
// an explicit monid), destroy, and the boolish listing decode.
func TestClient_CephMon_CreateDestroyList(t *testing.T) {
	listing := `{"data":[{"name":"pve1","addr":"10.0.0.1:6789/0","host":"pve1",` +
		`"state":"running","quorum":1,"rank":0,"service":1,"direxists":0,` +
		`"ceph_version":"ceph version 19.2.0","ceph_version_short":"19.2.0"}]}`
	var createBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/ceph/mon":
			_, _ = io.WriteString(w, listing)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/mon/pve1":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createBody)
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephcreatemon:root@pam:"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/ceph/mon/mon2":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createBody)
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephcreatemon:root@pam:"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/ceph/mon/pve1":
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:cephdestroymon:root@pam:"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	// Empty monid falls back to the node name in the URL per the pin's
	// monid default (nodename).
	upid, err := c.CreateCephMon(context.Background(), "pve1", "", "10.0.0.1")
	if err != nil {
		t.Fatalf("CreateCephMon: %v", err)
	}
	if !strings.Contains(upid, "cephcreatemon") {
		t.Fatalf("create upid = %q", upid)
	}
	if createBody["mon-address"] != "10.0.0.1" || createBody["monid"] != nil {
		t.Fatalf("create body = %v", createBody)
	}

	if _, err := c.CreateCephMon(context.Background(), "pve1", "mon2", ""); err != nil {
		t.Fatalf("CreateCephMon(mon2): %v", err)
	}
	if createBody["monid"] != "mon2" {
		t.Fatalf("create body = %v", createBody)
	}

	mons, err := c.ListCephMons(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListCephMons: %v", err)
	}
	if len(mons) != 1 || mons[0].Name != "pve1" || mons[0].Quorum == nil || !*mons[0].Quorum ||
		mons[0].Service == nil || !*mons[0].Service || mons[0].DirExists == nil || *mons[0].DirExists {
		t.Fatalf("mons = %+v", mons)
	}

	mon, err := c.GetCephMon(context.Background(), "pve1", "pve1")
	if err != nil {
		t.Fatalf("GetCephMon: %v", err)
	}
	if mon == nil || mon.State != "running" || mon.Rank == nil || *mon.Rank != 0 {
		t.Fatalf("mon = %+v", mon)
	}
	if _, err := c.GetCephMon(context.Background(), "pve1", "missing"); !isTestNotFound(err) {
		t.Fatalf("GetCephMon(missing) err = %v, want 404 APIError", err)
	}

	destroyed, err := c.DestroyCephMon(context.Background(), "pve1", "pve1")
	if err != nil {
		t.Fatalf("DestroyCephMon: %v", err)
	}
	if !strings.Contains(destroyed, "cephdestroymon") {
		t.Fatalf("destroy upid = %q", destroyed)
	}
}

// isTestNotFound reports whether err is an *APIError with status 404,
// mirroring the provider's isPVEClientNotFound.
func isTestNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// cephFloatPtr is a test helper for building expected *float64 values.
func cephFloatPtr(f float64) *float64 { return &f }
