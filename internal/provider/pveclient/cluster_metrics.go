// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MetricsServer mirrors one external metric server configuration as read
// from /cluster/metrics/server[/{id}] or written by POST/PUT
// /cluster/metrics/server/{id}. PVE stores metric servers in status.cfg as
// a section config: id, type, server, and port are required; every other
// field is optional (nil/"" = not set) and the pin scopes each to one or
// more plugin types (graphite, influxdb, opentelemetry), which the provider
// layer validates.
//
// Wire quirks normalized here:
//
//   - Booleans (disable, verify-certificate, otel-verify-ssl) arrive as 0/1
//     integers from status.cfg on most versions and as real booleans on
//     others.
//   - Numeric settings (port, mtu, the timeouts, max-body-sizes) may arrive
//     as JSON numbers or as strings, because the section config stores text
//     on disk.

type MetricsServer struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	Server  string `json:"server,omitempty"`
	Port    *int64 `json:"port,omitempty"`
	Disable *bool  `json:"disable,omitempty"`

	// Graphite plugin type.
	Proto   string `json:"proto,omitempty"`
	Path    string `json:"path,omitempty"`
	Timeout *int64 `json:"timeout,omitempty"`

	// InfluxDB plugin type.
	InfluxDBProto     string `json:"influxdbproto,omitempty"`
	Organization      string `json:"organization,omitempty"`
	Bucket            string `json:"bucket,omitempty"`
	Token             string `json:"token,omitempty"`
	APIPathPrefix     string `json:"api-path-prefix,omitempty"`
	MaxBodySize       *int64 `json:"max-body-size,omitempty"`
	VerifyCertificate *bool  `json:"verify-certificate,omitempty"`

	// OpenTelemetry plugin type.
	OTelCompression        string `json:"otel-compression,omitempty"`
	OTelHeaders            string `json:"otel-headers,omitempty"`
	OTelMaxBodySize        *int64 `json:"otel-max-body-size,omitempty"`
	OTelPath               string `json:"otel-path,omitempty"`
	OTelProtocol           string `json:"otel-protocol,omitempty"`
	OTelResourceAttributes string `json:"otel-resource-attributes,omitempty"`
	OTelTimeout            *int64 `json:"otel-timeout,omitempty"`
	OTelVerifySSL          *bool  `json:"otel-verify-ssl,omitempty"`

	// MTU applies to the UDP transports of graphite and influxdb.
	MTU *int64 `json:"mtu,omitempty"`

	// Digest is the read-only section config revision.
	Digest string `json:"digest,omitempty"`
}

// metricsServerRaw mirrors the wire shape with the lenient bool and integer
// fields left as raw JSON.
type metricsServerRaw struct {
	ID                     string          `json:"id"`
	Type                   string          `json:"type"`
	Server                 string          `json:"server"`
	Port                   json.RawMessage `json:"port"`
	Disable                json.RawMessage `json:"disable"`
	Proto                  string          `json:"proto"`
	Path                   string          `json:"path"`
	Timeout                json.RawMessage `json:"timeout"`
	InfluxDBProto          string          `json:"influxdbproto"`
	Organization           string          `json:"organization"`
	Bucket                 string          `json:"bucket"`
	Token                  string          `json:"token"`
	APIPathPrefix          string          `json:"api-path-prefix"`
	MaxBodySize            json.RawMessage `json:"max-body-size"`
	VerifyCertificate      json.RawMessage `json:"verify-certificate"`
	OTelCompression        string          `json:"otel-compression"`
	OTelHeaders            string          `json:"otel-headers"`
	OTelMaxBodySize        json.RawMessage `json:"otel-max-body-size"`
	OTelPath               string          `json:"otel-path"`
	OTelProtocol           string          `json:"otel-protocol"`
	OTelResourceAttributes string          `json:"otel-resource-attributes"`
	OTelTimeout            json.RawMessage `json:"otel-timeout"`
	OTelVerifySSL          json.RawMessage `json:"otel-verify-ssl"`
	MTU                    json.RawMessage `json:"mtu"`
	Digest                 string          `json:"digest"`
}

// UnmarshalJSON tolerates the 0/1 encoding of the boolean fields and the
// string encoding of numeric fields (see MetricsServer), keeping absent
// settings nil.
func (s *MetricsServer) UnmarshalJSON(data []byte) error {
	var raw metricsServerRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.ID = raw.ID
	s.Type = raw.Type
	s.Server = raw.Server
	s.Port = metricsServerInt64FromRaw(raw.Port)
	s.Disable = nodeNetworkBoolishPtr(raw.Disable)
	s.Proto = raw.Proto
	s.Path = raw.Path
	s.Timeout = metricsServerInt64FromRaw(raw.Timeout)
	s.InfluxDBProto = raw.InfluxDBProto
	s.Organization = raw.Organization
	s.Bucket = raw.Bucket
	s.Token = raw.Token
	s.APIPathPrefix = raw.APIPathPrefix
	s.MaxBodySize = metricsServerInt64FromRaw(raw.MaxBodySize)
	s.VerifyCertificate = nodeNetworkBoolishPtr(raw.VerifyCertificate)
	s.OTelCompression = raw.OTelCompression
	s.OTelHeaders = raw.OTelHeaders
	s.OTelMaxBodySize = metricsServerInt64FromRaw(raw.OTelMaxBodySize)
	s.OTelPath = raw.OTelPath
	s.OTelProtocol = raw.OTelProtocol
	s.OTelResourceAttributes = raw.OTelResourceAttributes
	s.OTelTimeout = metricsServerInt64FromRaw(raw.OTelTimeout)
	s.OTelVerifySSL = nodeNetworkBoolishPtr(raw.OTelVerifySSL)
	s.MTU = metricsServerInt64FromRaw(raw.MTU)
	s.Digest = raw.Digest
	return nil
}

// metricsServerInt64FromRaw decodes a lenient integer setting: PVE stores
// section config values as text on disk, so numeric metric server fields
// may arrive as JSON numbers or as JSON strings. Absent and null decode to
// nil, and a non-numeric value decodes to nil rather than failing the
// whole read.
func metricsServerInt64FromRaw(raw json.RawMessage) *int64 {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	trimmed = strings.Trim(trimmed, `"`)
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// ListMetricsServers returns the array from GET /cluster/metrics/server.
func (c *Client) ListMetricsServers(ctx context.Context) ([]MetricsServer, error) {
	var servers []MetricsServer
	if err := c.Do(ctx, "GET", "/cluster/metrics/server", nil, &servers); err != nil {
		return nil, fmt.Errorf("pveclient: list metric servers: %w", err)
	}
	return servers, nil
}

// CreateMetricsServer POSTs /cluster/metrics/server/{id} — the pin places
// create on the id leaf, not on the collection. ID, Type, Server, and Port
// are required per the pin.
func (c *Client) CreateMetricsServer(ctx context.Context, srv MetricsServer) error {
	if srv.ID == "" {
		return fmt.Errorf("pveclient: create metric server: id is required")
	}
	if srv.Type == "" {
		return fmt.Errorf("pveclient: create metric server %s: type is required", srv.ID)
	}
	if srv.Server == "" {
		return fmt.Errorf("pveclient: create metric server %s: server is required", srv.ID)
	}
	if srv.Port == nil {
		return fmt.Errorf("pveclient: create metric server %s: port is required", srv.ID)
	}
	if err := c.Do(ctx, "POST", "/cluster/metrics/server/"+srv.ID, srv, nil); err != nil {
		return fmt.Errorf("pveclient: create metric server %s: %w", srv.ID, err)
	}
	return nil
}

// GetMetricsServer reads /cluster/metrics/server/{id}.
func (c *Client) GetMetricsServer(ctx context.Context, id string) (*MetricsServer, error) {
	var srv MetricsServer
	if err := c.Do(ctx, "GET", "/cluster/metrics/server/"+id, nil, &srv); err != nil {
		return nil, fmt.Errorf("pveclient: read metric server %s: %w", id, err)
	}
	srv.ID = id
	return &srv, nil
}

// UpdateMetricsServer PUTs /cluster/metrics/server/{id} with the supplied
// fields and translates deleteFields into the PVE `delete` query parameter
// (comma-separated field names to clear). The pin's update verb carries no
// `type` parameter — the plugin type is immutable — so Type is never sent;
// `id` is a required PUT parameter and is forced to the path id.
func (c *Client) UpdateMetricsServer(ctx context.Context, id string, srv MetricsServer, deleteFields []string) error {
	srv.ID = id
	srv.Type = ""
	path := "/cluster/metrics/server/" + id
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	if err := c.Do(ctx, "PUT", path, srv, nil); err != nil {
		return fmt.Errorf("pveclient: update metric server %s: %w", id, err)
	}
	return nil
}

// DeleteMetricsServer deletes a metric server configuration. The mutation
// is synchronous per the pin (no task is spawned).
func (c *Client) DeleteMetricsServer(ctx context.Context, id string) error {
	if err := c.Do(ctx, "DELETE", "/cluster/metrics/server/"+id, nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete metric server %s: %w", id, err)
	}
	return nil
}
