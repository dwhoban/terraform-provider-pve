// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MappingDir is a directory hardware mapping (GET/POST
// /cluster/mapping/dir, GET/PUT/DELETE /cluster/mapping/dir/{id}). Map
// carries one entry per node; the wire encodes entries as PVE property
// strings (`node=<node>,path=<path>`).
type MappingDir struct {
	ID          string
	Description string
	Map         []MappingDirEntry
}

// MappingDirEntry is one node entry of a directory mapping.
type MappingDirEntry struct {
	Node string
	Path string
}

// MappingPCI is a PCI hardware mapping (GET/POST /cluster/mapping/pci,
// GET/PUT/DELETE /cluster/mapping/pci/{id}). Mdev and
// LiveMigrationCapable tolerate the 0/1 int encoding on decode.
type MappingPCI struct {
	ID                   string
	Description          string
	Mdev                 *bool
	LiveMigrationCapable *bool
	Map                  []MappingPCIEntry
}

// MappingPCIEntry is one node entry of a PCI mapping.
type MappingPCIEntry struct {
	Node        string
	ID          string
	IOMMUGroup  *int64
	Path        string
	SubsystemID string
	Description string
}

// MappingUSB is a USB hardware mapping (GET/POST /cluster/mapping/usb,
// GET/PUT/DELETE /cluster/mapping/usb/{id}).
type MappingUSB struct {
	ID          string
	Description string
	Map         []MappingUSBEntry
}

// MappingUSBEntry is one node entry of a USB mapping.
type MappingUSBEntry struct {
	Node        string
	ID          string
	Path        string
	Description string
}

// mappingEncodePropertyString encodes key/value fields as one PVE property
// string, quoting values that contain separators and dropping empty
// optional fields.
func mappingEncodePropertyString(fields [][2]string) string {
	parts := make([]string, 0, len(fields))
	for _, kv := range fields {
		v := kv[1]
		if v == "" {
			continue
		}
		if strings.ContainsAny(v, ",\"= \\") {
			v = "\"" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\"", "\\\"") + "\""
		}
		parts = append(parts, kv[0]+"="+v)
	}
	return strings.Join(parts, ",")
}

// mappingParsePropertyString decodes one PVE property string into its
// fields, honoring quoted values and backslash escapes.
func mappingParsePropertyString(s string) map[string]string {
	out := map[string]string{}
	i, n := 0, len(s)
	for i < n {
		var key strings.Builder
		for i < n && s[i] != '=' {
			key.WriteByte(s[i])
			i++
		}
		if i >= n {
			break
		}
		i++ // skip '='
		var val strings.Builder
		inQuotes := false
		for i < n {
			ch := s[i]
			if ch == '\\' && i+1 < n {
				val.WriteByte(s[i+1])
				i += 2
				continue
			}
			if ch == '"' {
				inQuotes = !inQuotes
				i++
				continue
			}
			if ch == ',' && !inQuotes {
				i++
				break
			}
			val.WriteByte(ch)
			i++
		}
		out[key.String()] = val.String()
	}
	return out
}

// mappingDirWire mirrors the wire shape of MappingDir.
type mappingDirWire struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Map         []string `json:"map"`
}

// MarshalJSON encodes the map entries as property strings.
func (m MappingDir) MarshalJSON() ([]byte, error) {
	wire := mappingDirWire{ID: m.ID, Description: m.Description}
	for _, e := range m.Map {
		wire.Map = append(wire.Map, mappingEncodePropertyString([][2]string{{"node", e.Node}, {"path", e.Path}}))
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the property-string map entries.
func (m *MappingDir) UnmarshalJSON(data []byte) error {
	var wire mappingDirWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.ID, m.Description = wire.ID, wire.Description
	m.Map = make([]MappingDirEntry, 0, len(wire.Map))
	for _, s := range wire.Map {
		f := mappingParsePropertyString(s)
		m.Map = append(m.Map, MappingDirEntry{Node: f["node"], Path: f["path"]})
	}
	return nil
}

// mappingPCIWire mirrors the wire shape of MappingPCI with the lenient
// fields as json.RawMessage.
type mappingPCIWire struct {
	ID                   string          `json:"id"`
	Description          string          `json:"description"`
	Mdev                 json.RawMessage `json:"mdev"`
	LiveMigrationCapable json.RawMessage `json:"live-migration-capable"`
	Map                  []string        `json:"map"`
}

// MarshalJSON encodes the map entries as property strings and the flags as
// the pin's hyphenated wire keys.
func (m MappingPCI) MarshalJSON() ([]byte, error) {
	wire := mappingPCIWire{ID: m.ID, Description: m.Description}
	if m.Mdev != nil {
		wire.Mdev = haBoolRaw(*m.Mdev)
	}
	if m.LiveMigrationCapable != nil {
		wire.LiveMigrationCapable = haBoolRaw(*m.LiveMigrationCapable)
	}
	for _, e := range m.Map {
		iommu := ""
		if e.IOMMUGroup != nil {
			iommu = fmt.Sprintf("%d", *e.IOMMUGroup)
		}
		wire.Map = append(wire.Map, mappingEncodePropertyString([][2]string{
			{"node", e.Node},
			{"id", e.ID},
			{"iommugroup", iommu},
			{"path", e.Path},
			{"subsystem-id", e.SubsystemID},
			{"description", e.Description},
		}))
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the boolish flags and the property-string map
// entries.
func (m *MappingPCI) UnmarshalJSON(data []byte) error {
	var wire mappingPCIWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.ID, m.Description = wire.ID, wire.Description
	m.Mdev = decodeBoolishPtr(wire.Mdev)
	m.LiveMigrationCapable = decodeBoolishPtr(wire.LiveMigrationCapable)
	m.Map = make([]MappingPCIEntry, 0, len(wire.Map))
	for _, s := range wire.Map {
		f := mappingParsePropertyString(s)
		entry := MappingPCIEntry{
			Node:        f["node"],
			ID:          f["id"],
			Path:        f["path"],
			SubsystemID: f["subsystem-id"],
			Description: f["description"],
		}
		if f["iommugroup"] != "" {
			var group int64
			if _, err := fmt.Sscanf(f["iommugroup"], "%d", &group); err == nil {
				entry.IOMMUGroup = &group
			}
		}
		m.Map = append(m.Map, entry)
	}
	return nil
}

// mappingUSBWire mirrors the wire shape of MappingUSB.
type mappingUSBWire struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Map         []string `json:"map"`
}

// MarshalJSON encodes the map entries as property strings.
func (m MappingUSB) MarshalJSON() ([]byte, error) {
	wire := mappingUSBWire{ID: m.ID, Description: m.Description}
	for _, e := range m.Map {
		wire.Map = append(wire.Map, mappingEncodePropertyString([][2]string{
			{"node", e.Node},
			{"id", e.ID},
			{"path", e.Path},
			{"description", e.Description},
		}))
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the property-string map entries.
func (m *MappingUSB) UnmarshalJSON(data []byte) error {
	var wire mappingUSBWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.ID, m.Description = wire.ID, wire.Description
	m.Map = make([]MappingUSBEntry, 0, len(wire.Map))
	for _, s := range wire.Map {
		f := mappingParsePropertyString(s)
		m.Map = append(m.Map, MappingUSBEntry{Node: f["node"], ID: f["id"], Path: f["path"], Description: f["description"]})
	}
	return nil
}

// ListMappingDirs returns the directory mappings from GET
// /cluster/mapping/dir.
func (c *Client) ListMappingDirs(ctx context.Context) ([]MappingDir, error) {
	var out []MappingDir
	if err := c.Do(ctx, "GET", "/cluster/mapping/dir", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateMappingDir POSTs /cluster/mapping/dir with the supplied mapping.
func (c *Client) CreateMappingDir(ctx context.Context, m MappingDir) error {
	return c.Do(ctx, "POST", "/cluster/mapping/dir", m, nil)
}

// GetMappingDir reads /cluster/mapping/dir/{id}.
func (c *Client) GetMappingDir(ctx context.Context, id string) (*MappingDir, error) {
	var out MappingDir
	path := fmt.Sprintf("/cluster/mapping/dir/%s", id)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMappingDir PUTs /cluster/mapping/dir/{id} with the supplied body
// and translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateMappingDir(ctx context.Context, id string, m MappingDir, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/mapping/dir/%s", id), deleteFields)
	return c.Do(ctx, "PUT", path, m, nil)
}

// DeleteMappingDir DELETEs /cluster/mapping/dir/{id}.
func (c *Client) DeleteMappingDir(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/mapping/dir/%s", id), nil, nil)
}

// ListMappingPCI returns the PCI mappings from GET /cluster/mapping/pci.
func (c *Client) ListMappingPCI(ctx context.Context) ([]MappingPCI, error) {
	var out []MappingPCI
	if err := c.Do(ctx, "GET", "/cluster/mapping/pci", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateMappingPCI POSTs /cluster/mapping/pci with the supplied mapping.
func (c *Client) CreateMappingPCI(ctx context.Context, m MappingPCI) error {
	return c.Do(ctx, "POST", "/cluster/mapping/pci", m, nil)
}

// GetMappingPCI reads /cluster/mapping/pci/{id}.
func (c *Client) GetMappingPCI(ctx context.Context, id string) (*MappingPCI, error) {
	var out MappingPCI
	path := fmt.Sprintf("/cluster/mapping/pci/%s", id)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMappingPCI PUTs /cluster/mapping/pci/{id} with the supplied body
// and translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateMappingPCI(ctx context.Context, id string, m MappingPCI, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/mapping/pci/%s", id), deleteFields)
	return c.Do(ctx, "PUT", path, m, nil)
}

// DeleteMappingPCI DELETEs /cluster/mapping/pci/{id}.
func (c *Client) DeleteMappingPCI(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/mapping/pci/%s", id), nil, nil)
}

// ListMappingUSB returns the USB mappings from GET /cluster/mapping/usb.
func (c *Client) ListMappingUSB(ctx context.Context) ([]MappingUSB, error) {
	var out []MappingUSB
	if err := c.Do(ctx, "GET", "/cluster/mapping/usb", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateMappingUSB POSTs /cluster/mapping/usb with the supplied mapping.
func (c *Client) CreateMappingUSB(ctx context.Context, m MappingUSB) error {
	return c.Do(ctx, "POST", "/cluster/mapping/usb", m, nil)
}

// GetMappingUSB reads /cluster/mapping/usb/{id}.
func (c *Client) GetMappingUSB(ctx context.Context, id string) (*MappingUSB, error) {
	var out MappingUSB
	path := fmt.Sprintf("/cluster/mapping/usb/%s", id)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMappingUSB PUTs /cluster/mapping/usb/{id} with the supplied body
// and translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateMappingUSB(ctx context.Context, id string, m MappingUSB, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/mapping/usb/%s", id), deleteFields)
	return c.Do(ctx, "PUT", path, m, nil)
}

// DeleteMappingUSB DELETEs /cluster/mapping/usb/{id}.
func (c *Client) DeleteMappingUSB(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/mapping/usb/%s", id), nil, nil)
}
