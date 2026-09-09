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
	"sync/atomic"
	"testing"
)

// TestClient_UploadStorageContent_WireBody asserts the multipart wire shape
// of POST /nodes/{node}/storage/{storage}/upload: a multipart/form-data body
// whose boundary appears in the Content-Type header, a `content` form field
// carrying the content type, and a `filename` file part carrying the target
// name and the raw bytes.
func TestClient_UploadStorageContent_WireBody(t *testing.T) {
	var sawContentType, sawContent, sawFilename, sawChecksum, sawAlgorithm string
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/storage/local/upload" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		sawContent = r.FormValue("content")
		sawChecksum = r.FormValue("checksum")
		sawAlgorithm = r.FormValue("checksum-algorithm")
		file, header, err := r.FormFile("filename")
		if err != nil {
			t.Fatalf("multipart file part `filename`: %v", err)
		}
		defer file.Close()
		sawFilename = header.Filename
		sawBody, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file part: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0000ABCD:12345678:upload:local:dn-12.iso"}`)
	})
	checksum := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	upid, err := c.UploadStorageContent(context.Background(), "pve1", "local", "debian-12.iso", "iso", []byte("fake-iso-bytes"), &checksum, "sha256")
	if err != nil {
		t.Fatalf("UploadStorageContent: %v", err)
	}
	if upid != "UPID:pve1:0000ABCD:12345678:upload:local:dn-12.iso" {
		t.Fatalf("upid = %q", upid)
	}
	if !strings.HasPrefix(sawContentType, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/form-data with boundary", sawContentType)
	}
	if sawContent != "iso" {
		t.Fatalf("content field = %q, want iso", sawContent)
	}
	if sawFilename != "debian-12.iso" {
		t.Fatalf("filename part name = %q, want debian-12.iso", sawFilename)
	}
	if string(sawBody) != "fake-iso-bytes" {
		t.Fatalf("file part body = %q, want fake-iso-bytes", sawBody)
	}
	if sawChecksum != checksum || sawAlgorithm != "sha256" {
		t.Fatalf("checksum = %q algorithm = %q", sawChecksum, sawAlgorithm)
	}
}

// TestClient_UploadStorageContent_NoChecksumOmitted keeps the checksum pair
// out of the multipart body entirely when unset.
func TestClient_UploadStorageContent_NoChecksumOmitted(t *testing.T) {
	var sawForm bool
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		_, sawChecksum := r.MultipartForm.Value["checksum"]
		_, sawAlgorithm := r.MultipartForm.Value["checksum-algorithm"]
		if sawChecksum || sawAlgorithm {
			t.Fatalf("checksum fields sent without configuration: %v", r.MultipartForm.Value)
		}
		sawForm = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:upload:local:x.iso"}`)
	})
	if _, err := c.UploadStorageContent(context.Background(), "pve1", "local", "x.iso", "iso", []byte("x"), nil, ""); err != nil {
		t.Fatalf("UploadStorageContent: %v", err)
	}
	if !sawForm {
		t.Fatal("handler never ran")
	}
}

// TestClient_UploadStorageContent_HalfChecksumRejected refuses the request
// when only one half of the mutually-required checksum pair is set; the pin
// requires checksum and checksum-algorithm together.
func TestClient_UploadStorageContent_HalfChecksumRejected(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request should be sent for a half-specified checksum")
	})
	if _, err := c.UploadStorageContent(context.Background(), "pve1", "local", "x.iso", "iso", []byte("x"), nil, "sha256"); err == nil {
		t.Fatal("expected error for algorithm without checksum, got nil")
	}
	checksum := "abc"
	if _, err := c.UploadStorageContent(context.Background(), "pve1", "local", "x.iso", "iso", []byte("x"), &checksum, ""); err == nil {
		t.Fatal("expected error for checksum without algorithm, got nil")
	}
}

// TestClient_DownloadStorageContentURL_WireBody asserts the JSON body of
// POST /nodes/{node}/storage/{storage}/download-url, including the
// hyphenated pin keys and the returned task UPID.
func TestClient_DownloadStorageContentURL_WireBody(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/storage/local/download-url" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0000ABCD:12345678:download:local:dn-12.iso"}`)
	})
	insecure := true
	upid, err := c.DownloadStorageContentURL(context.Background(), "pve1", "local", "https://example.com/debian-12.iso", "debian-12.iso", "iso", &StorageContentDownloadURLOptions{
		Checksum:           strPtrStorageContent("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"),
		ChecksumAlgorithm:  strPtrStorageContent("sha256"),
		VerifyCertificates: &insecure,
	})
	if err != nil {
		t.Fatalf("DownloadStorageContentURL: %v", err)
	}
	if upid != "UPID:pve1:0000ABCD:12345678:download:local:dn-12.iso" {
		t.Fatalf("upid = %q", upid)
	}
	if sawBody["url"] != "https://example.com/debian-12.iso" || sawBody["content"] != "iso" || sawBody["filename"] != "debian-12.iso" {
		t.Fatalf("body = %v", sawBody)
	}
	if sawBody["checksum-algorithm"] != "sha256" {
		t.Fatalf("checksum-algorithm = %v", sawBody["checksum-algorithm"])
	}
	if sawBody["verify-certificates"] != true {
		t.Fatalf("verify-certificates = %v", sawBody["verify-certificates"])
	}
}

// TestClient_DownloadStorageContentURL_MinimalBody sends only the three
// required parameters when every option is unset.
func TestClient_DownloadStorageContentURL_MinimalBody(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:download:local:x.iso"}`)
	})
	if _, err := c.DownloadStorageContentURL(context.Background(), "pve1", "local", "https://example.com/x.iso", "x.iso", "iso", nil); err != nil {
		t.Fatalf("DownloadStorageContentURL: %v", err)
	}
	if len(sawBody) != 3 {
		t.Fatalf("body = %v, want exactly url, content and filename", sawBody)
	}
}

// TestClient_GetStorageContentEntry decodes GET
// /nodes/{node}/storage/{storage}/content/{volume}; the volume segment must
// keep the volid's `/` percent-escaped on the wire so upstream path
// splitting sees a single segment (Go leaves the `:` of a path segment
// unescaped, which PVE accepts).
func TestClient_GetStorageContentEntry(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/storage/local/content/local:iso/debian-12.iso" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.URL.EscapedPath() != "/nodes/pve1/storage/local/content/local:iso%2Fdebian-12.iso" {
			t.Fatalf("volume not escaped on the wire: %s", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"format":"iso","notes":"installer","path":"/var/lib/vz/template/iso/debian-12.iso","protected":0,"size":631000000,"used":631000000}}`)
	})
	entry, err := c.GetStorageContentEntry(context.Background(), "pve1", "local", "local:iso/debian-12.iso")
	if err != nil {
		t.Fatalf("GetStorageContentEntry: %v", err)
	}
	if entry == nil || entry.Format != "iso" || entry.Path != "/var/lib/vz/template/iso/debian-12.iso" {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Size == nil || *entry.Size != 631000000 || entry.Used == nil || *entry.Used != 631000000 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Notes == nil || *entry.Notes != "installer" {
		t.Fatalf("entry = %+v", entry)
	}
}

// TestClient_GetStorageContentEntry_404IsAPIError surfaces a missing volume
// as *APIError so the provider layer can distinguish absent resources. The
// helper wraps the error, so the assertion unwraps via errors.As.
func TestClient_GetStorageContentEntry_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"volume 'local:iso/gone.iso' does not exist"}`)
	})
	_, err := c.GetStorageContentEntry(context.Background(), "pve1", "local", "local:iso/gone.iso")
	if err == nil {
		t.Fatal("expected error for missing volume, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", apiErr.StatusCode)
	}
}

// TestClient_DeleteStorageContent_WaitsForTask asserts DELETE
// /nodes/{node}/storage/{storage}/content/{volume} with the volid's `/`
// percent-escaped on the wire, and that a returned task UPID is awaited via
// the task status endpoint.
func TestClient_DeleteStorageContent_WaitsForTask(t *testing.T) {
	var polls int32
	upid := "UPID:pve1:0000ABCD:12345678:unlink:local:x.tar.gz"
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/storage/local/content/local:vztmpl/x.tar.gz":
			if r.URL.EscapedPath() != "/nodes/pve1/storage/local/content/local:vztmpl%2Fx.tar.gz" {
				t.Fatalf("volume not escaped on the wire: %s", r.URL.EscapedPath())
			}
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			if atomic.AddInt32(&polls, 1) < 2 {
				_, _ = io.WriteString(w, `{"data":{"status":"running","exitstatus":"","pid":42}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK","pid":42}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err := c.DeleteStorageContent(context.Background(), "pve1", "local", "local:vztmpl/x.tar.gz"); err != nil {
		t.Fatalf("DeleteStorageContent: %v", err)
	}
	if got := atomic.LoadInt32(&polls); got < 2 {
		t.Fatalf("polls = %d, want >= 2", got)
	}
}

// TestClient_DeleteStorageContent_NullUPIDDoesNotPoll covers the pin's
// optional return: when the deletion finishes inline, `data` is null and no
// task status endpoint is contacted.
func TestClient_DeleteStorageContent_NullUPIDDoesNotPoll(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/storage/local/content/local:iso/x.iso" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if strings.Contains(r.URL.Path, "/tasks/") {
			t.Fatal("no task polling expected for an inline deletion")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteStorageContent(context.Background(), "pve1", "local", "local:iso/x.iso"); err != nil {
		t.Fatalf("DeleteStorageContent: %v", err)
	}
}

// strPtrStorageContent is a local helper for building expected *string
// values in the storage content tests.
func strPtrStorageContent(s string) *string { return &s }
