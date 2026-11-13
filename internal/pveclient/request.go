// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

// DoRequest performs one authenticated request against the PVE API and
// returns the decoded "data" payload of the /api2/json envelope. It is the
// generic escape hatch behind the generated provider types: params are
// query-encoded for GET/DELETE and form-encoded for POST/PUT. The payload is
// nil when the endpoint answers with a null data field; callers inspect the
// raw JSON shape (string UPID, object, or array) themselves.
func (c *Client) DoRequest(ctx context.Context, method, path string, params url.Values) (json.RawMessage, error) {
	payload, err := c.doRaw(ctx, method, path, params, nil)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		return nil, nil
	}
	return payload, nil
}

// notFoundMessageRe matches the phrasings the PVE API uses inside HTTP 500
// responses when the addressed object does not exist.
var notFoundMessageRe = regexp.MustCompile(`(?i)\b(no such|not found|does not exist)\b`)

// IsNotFound reports whether err represents an object that does not exist.
// The PVE API signals missing objects inconsistently: sometimes HTTP 404,
// but usually HTTP 500 carrying a message such as "no such storage" or
// "configuration file ... does not exist". Both spellings are treated as
// not-found; the pattern is deliberately narrow so unrelated server errors
// are not swallowed.
func IsNotFound(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	if ae.StatusCode == http.StatusNotFound {
		return true
	}
	return ae.StatusCode >= http.StatusInternalServerError && notFoundMessageRe.MatchString(ae.Message)
}
