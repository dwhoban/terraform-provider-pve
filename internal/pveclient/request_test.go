// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDoRequestEnvelopeShapes(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string // expected raw payload, "" for nil
		wantErr string
	}{
		{name: "object", body: `{"data":{"a":1}}`, want: `{"a":1}`},
		{name: "array", body: `{"data":[1,2]}`, want: `[1,2]`},
		{name: "upid string", body: `{"data":"UPID:n1:0000:0:0:qmcreate:100:"}`, want: `"UPID:n1:0000:0:0:qmcreate:100:"`},
		{name: "null data", body: `{"data":null}`, want: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, Credentials{APIToken: "u@pam!t=uuid"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, tt.body)
			}))
			payload, err := c.DoRequest(context.Background(), http.MethodGet, "/anything", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := string(payload); got != tt.want {
				t.Fatalf("payload = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDoRequestParamsEncodings(t *testing.T) {
	var gotMethod, gotQuery, gotBody string
	var gotForm url.Values
	c := testClient(t, Credentials{APIToken: "u@pam!t=uuid"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			gotForm, _ = url.ParseQuery(gotBody)
		}
		writeJSON(w, http.StatusOK, `{"data":null}`)
	}))
	params := url.Values{"a": []string{"1"}, "b": []string{"x y"}}

	if _, err := c.DoRequest(context.Background(), http.MethodGet, "/p", params); err != nil {
		t.Fatalf("GET: %v", err)
	}
	if gotMethod != http.MethodGet || gotQuery != "a=1&b=x+y" || gotBody != "" {
		t.Fatalf("GET encoded wrong: method=%s query=%q body=%q", gotMethod, gotQuery, gotBody)
	}

	if _, err := c.DoRequest(context.Background(), http.MethodPost, "/p", params); err != nil {
		t.Fatalf("POST: %v", err)
	}
	if gotMethod != http.MethodPost || gotQuery != "" || gotForm.Get("a") != "1" || gotForm.Get("b") != "x y" {
		t.Fatalf("POST encoded wrong: method=%s query=%q form=%v", gotMethod, gotQuery, gotForm)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "404", err: &APIError{StatusCode: 404, Status: "404 Not Found", Message: "not found here"}, want: true},
		{name: "500 no such", err: &APIError{StatusCode: 500, Status: "500", Message: "no such storage 'x'"}, want: true},
		{name: "500 does not exist", err: &APIError{StatusCode: 500, Status: "500", Message: "Configuration file 'qemu/100.conf' does not exist"}, want: true},
		{name: "500 other", err: &APIError{StatusCode: 500, Status: "500", Message: "internal error"}, want: false},
		{name: "403", err: &APIError{StatusCode: 403, Status: "403", Message: "no such permission"}, want: false},
		{name: "wrapped", err: wrapErr(&APIError{StatusCode: 500, Status: "500", Message: "no such guest"}), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Fatalf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func wrapErr(err error) error { return &wrapped{err} }

type wrapped struct{ error }

func (w *wrapped) Unwrap() error { return w.error }

var _ = strings.Contains // retained for future assertions
