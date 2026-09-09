// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

// StorageContentEntry mirrors GET /nodes/{node}/storage/{storage}/content/{volume}
// ("Get volume attributes"). Format, Path, Size and Used describe the volume
// itself; Notes and Protected are optional attributes settable through the
// volume's update-attributes endpoint. Listing-shaped reads (which also
// carry volid and ctime) go through ListStorageContent instead.
type StorageContentEntry struct {
	Format    string  `json:"format"`
	Notes     *string `json:"notes,omitempty"`
	Path      string  `json:"path"`
	Protected *bool   `json:"protected,omitempty"`
	Size      *int64  `json:"size,omitempty"`
	Used      *int64  `json:"used,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of `protected` that PVE
// emits, alongside the JSON boolean form.
func (e *StorageContentEntry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format    string          `json:"format"`
		Notes     *string         `json:"notes,omitempty"`
		Path      string          `json:"path"`
		Protected json.RawMessage `json:"protected,omitempty"`
		Size      *int64          `json:"size,omitempty"`
		Used      *int64          `json:"used,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Format = raw.Format
	e.Notes = raw.Notes
	e.Path = raw.Path
	e.Size = raw.Size
	e.Used = raw.Used
	e.Protected = decodeBoolishPtr(raw.Protected)
	return nil
}

// StorageContentDownloadURLOptions carries the optional parameters of POST
// /nodes/{node}/storage/{storage}/download-url. The checksum pair is only
// valid together (the pin marks each as requiring the other); a half-set
// pair is rejected before any request is sent. A nil *VerifyCertificates
// keeps the PVE default of verifying TLS certificates.
type StorageContentDownloadURLOptions struct {
	Checksum           *string
	ChecksumAlgorithm  *string
	Compression        *string
	VerifyCertificates *bool
}

// storageContentDownloadURLBody is the JSON request shape; the hyphenated
// keys follow the pin verbatim.
type storageContentDownloadURLBody struct {
	URL                string  `json:"url"`
	Content            string  `json:"content"`
	Filename           string  `json:"filename"`
	Checksum           *string `json:"checksum,omitempty"`
	ChecksumAlgorithm  *string `json:"checksum-algorithm,omitempty"`
	Compression        *string `json:"compression,omitempty"`
	VerifyCertificates *bool   `json:"verify-certificates,omitempty"`
}

// storageContentPath builds the escaped /content/{volume} tail of the
// storage content endpoints. Voids contain `:` and `/`, so the volume
// segment must be percent-escaped to survive path splitting upstream.
func storageContentPath(node, storage, volid string) string {
	return "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/content/" + url.PathEscape(volid)
}

// GetStorageContentEntry reads GET
// /nodes/{node}/storage/{storage}/content/{volume}. A missing volume
// surfaces as *APIError with a 404 status.
func (c *Client) GetStorageContentEntry(ctx context.Context, node, storage, volid string) (*StorageContentEntry, error) {
	var entry StorageContentEntry
	if err := c.Do(ctx, http.MethodGet, storageContentPath(node, storage, volid), nil, &entry); err != nil {
		return nil, fmt.Errorf("getting storage content entry %s on %s/%s: %w", volid, node, storage, err)
	}
	return &entry, nil
}

// UploadStorageContent POSTs the multipart body of
// /nodes/{node}/storage/{storage}/upload and returns the resulting task
// UPID; the caller waits on it with WaitForTask. The file travels as the
// multipart `filename` file part (PVE derives both the target name and the
// temp file from it), the content type as the `content` form field. The
// checksum pair is sent only when both halves are set; a half-set pair is
// rejected because the pin marks checksum and checksum-algorithm as
// mutually requiring. Note that the client's per-request HTTP timeout also
// bounds the transfer of large files.
func (c *Client) UploadStorageContent(ctx context.Context, node, storage, filename, contentType string, content []byte, checksum *string, checksumAlgorithm string) (string, error) {
	// The pin requires checksum and checksum-algorithm together; fail fast
	// instead of letting PVE reject the upload halfway through.
	haveChecksum := checksum != nil
	haveAlgorithm := checksumAlgorithm != ""
	if haveChecksum != haveAlgorithm {
		return "", fmt.Errorf("uploading %s to %s/%s: checksum and checksum-algorithm must be set together", filename, node, storage)
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("content", contentType); err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: build multipart body: %w", filename, node, storage, err)
	}
	if haveChecksum {
		if err := writer.WriteField("checksum", *checksum); err != nil {
			return "", fmt.Errorf("uploading %s to %s/%s: build multipart body: %w", filename, node, storage, err)
		}
		if err := writer.WriteField("checksum-algorithm", checksumAlgorithm); err != nil {
			return "", fmt.Errorf("uploading %s to %s/%s: build multipart body: %w", filename, node, storage, err)
		}
	}
	part, err := writer.CreateFormFile("filename", filename)
	if err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: build multipart body: %w", filename, node, storage, err)
	}
	if _, err := part.Write(content); err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: write file part: %w", filename, node, storage, err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: close multipart body: %w", filename, node, storage, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/nodes/"+url.PathEscape(node)+"/storage/"+url.PathEscape(storage)+"/upload", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: build request: %w", filename, node, storage, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	if err := c.sign(req); err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: %w", filename, node, storage, err)
	}
	upid, err := c.doEnvelope(req)
	if err != nil {
		return "", fmt.Errorf("uploading %s to %s/%s: %w", filename, node, storage, err)
	}
	return upid, nil
}

// DownloadStorageContentURL POSTs /nodes/{node}/storage/{storage}/download-url
// and returns the resulting download task UPID; the caller waits on it with
// WaitForTask. PVE performs the transfer itself, so no file bytes travel
// through this client. The checksum pair is rejected when only half-set,
// mirroring the upload helper.
func (c *Client) DownloadStorageContentURL(ctx context.Context, node, storage, fileURL, filename, contentType string, opts *StorageContentDownloadURLOptions) (string, error) {
	body := storageContentDownloadURLBody{
		URL:      fileURL,
		Content:  contentType,
		Filename: filename,
	}
	if opts != nil {
		if (opts.Checksum != nil) != (opts.ChecksumAlgorithm != nil) {
			return "", fmt.Errorf("downloading %s to %s/%s: checksum and checksum-algorithm must be set together", filename, node, storage)
		}
		body.Checksum = opts.Checksum
		body.ChecksumAlgorithm = opts.ChecksumAlgorithm
		body.Compression = opts.Compression
		body.VerifyCertificates = opts.VerifyCertificates
	}
	var upid string
	if err := c.Do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/storage/"+url.PathEscape(storage)+"/download-url", body, &upid); err != nil {
		return "", fmt.Errorf("downloading %s to %s/%s: %w", filename, node, storage, err)
	}
	return upid, nil
}

// DeleteStorageContent DELETEs /nodes/{node}/storage/{storage}/content/{volume}.
// PVE runs the deletion as a worker: when the call returns a task UPID the
// deletion is still in flight, so it is awaited here; a null result means
// the volume is already gone and there is nothing to wait for.
func (c *Client) DeleteStorageContent(ctx context.Context, node, storage, volid string) error {
	var upid string
	if err := c.Do(ctx, http.MethodDelete, storageContentPath(node, storage, volid), nil, &upid); err != nil {
		return fmt.Errorf("deleting storage content %s on %s/%s: %w", volid, node, storage, err)
	}
	if upid == "" {
		return nil
	}
	if _, err := c.WaitForTask(ctx, node, upid, WaitForTaskOptions{}); err != nil {
		return fmt.Errorf("deleting storage content %s on %s/%s: %w", volid, node, storage, err)
	}
	return nil
}

// doEnvelope sends a pre-built request and decodes the standard PVE
// {"data": ...} envelope, mirroring the status handling of Do. It exists so
// the multipart upload can reuse the envelope and error semantics without
// routing its body through Do's JSON marshaller.
func (c *Client) doEnvelope(req *http.Request) (string, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", fmt.Errorf("read response %s %s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode, Method: req.Method, Path: req.URL.Path}
		var env envelope
		if jerr := json.Unmarshal(raw, &env); jerr == nil && len(env.Errors) > 0 {
			var s string
			if serr := json.Unmarshal(env.Errors, &s); serr == nil {
				apiErr.Errors = []string{s}
			} else {
				_ = json.Unmarshal(env.Errors, &apiErr.Errors)
			}
		}
		if len(apiErr.Errors) == 0 {
			apiErr.Errors = []string{string(raw)}
		}
		return "", apiErr
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("decode envelope %s %s: %w", req.Method, req.URL.Path, err)
	}
	if len(env.Data) == 0 {
		return "", nil
	}
	var upid string
	if err := json.Unmarshal(env.Data, &upid); err != nil {
		return "", fmt.Errorf("decode data %s %s: %w", req.Method, req.URL.Path, err)
	}
	return upid, nil
}
