// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AcmeAccountRegistration is the body of POST /cluster/acme/account
// (register_account). Contact entries are joined into PVE's comma-separated
// email-list wire string; the EAB keys use the pin's hyphenated parameter
// names.
type AcmeAccountRegistration struct {
	Name       string
	Contact    []string
	Directory  string
	TosURL     string
	EabKid     string
	EabHMACKey string
}

// MarshalJSON encodes the registration with PVE's wire keys.
func (r AcmeAccountRegistration) MarshalJSON() ([]byte, error) {
	wire := map[string]any{}
	if r.Name != "" {
		wire["name"] = r.Name
	}
	if len(r.Contact) > 0 {
		wire["contact"] = strings.Join(r.Contact, ",")
	}
	if r.Directory != "" {
		wire["directory"] = r.Directory
	}
	if r.TosURL != "" {
		wire["tos_url"] = r.TosURL
	}
	if r.EabKid != "" {
		wire["eab-kid"] = r.EabKid
	}
	if r.EabHMACKey != "" {
		wire["eab-hmac-key"] = r.EabHMACKey
	}
	return json.Marshal(wire)
}

// AcmeAccount is the response of GET /cluster/acme/account/{name}. The
// account field is the opaque ACME account document (the pin declares no
// properties), so it is surfaced verbatim; Location is the CA account URL.
type AcmeAccount struct {
	Account   json.RawMessage `json:"account,omitempty"`
	Directory string          `json:"directory,omitempty"`
	Location  string          `json:"location,omitempty"`
	Tos       string          `json:"tos,omitempty"`
}

// AcmePlugin is an ACME challenge plugin (GET/POST /cluster/acme/plugins,
// GET/PUT/DELETE /cluster/acme/plugins/{id}). The wire names the identifier
// `id` on create and `plugin` on responses; Nodes carries its list entries
// and is encoded as PVE's comma-separated wire string. Disable tolerates the
// 0/1 int encoding on decode.
type AcmePlugin struct {
	Plugin          string
	Type            string
	API             string
	Data            string
	Disable         *bool
	Nodes           []string
	ValidationDelay *int64
	Digest          string
}

// acmePluginWire mirrors the wire shape of AcmePlugin.
type acmePluginWire struct {
	Plugin          string          `json:"plugin"`
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	API             string          `json:"api"`
	Data            string          `json:"data"`
	Disable         json.RawMessage `json:"disable"`
	Nodes           string          `json:"nodes"`
	ValidationDelay *int64          `json:"validation-delay"`
	Digest          string          `json:"digest"`
}

// MarshalJSON encodes the plugin for create and update, emitting the create
// identifier as `id` and the node list as a comma-separated wire string.
func (p AcmePlugin) MarshalJSON() ([]byte, error) {
	wire := acmePluginWire{
		Plugin:          p.Plugin,
		ID:              p.Plugin,
		Type:            p.Type,
		API:             p.API,
		Data:            p.Data,
		Nodes:           strings.Join(p.Nodes, ","),
		ValidationDelay: p.ValidationDelay,
		Digest:          p.Digest,
	}
	if p.Disable != nil {
		wire.Disable = haBoolRaw(*p.Disable)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	// Drop empty keys so create/update bodies only carry what is set.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	for k, v := range generic {
		if string(v) == `""` || string(v) == "null" {
			delete(generic, k)
		}
	}
	return json.Marshal(generic)
}

// UnmarshalJSON decodes the plugin response, splitting the wire node list
// and tolerating the 0/1 int encoding of the disable flag.
func (p *AcmePlugin) UnmarshalJSON(data []byte) error {
	var wire acmePluginWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*p = AcmePlugin{
		Plugin:          wire.Plugin,
		Type:            wire.Type,
		API:             wire.API,
		Data:            wire.Data,
		Disable:         decodeBoolishPtr(wire.Disable),
		Nodes:           haSplitList(wire.Nodes),
		ValidationDelay: wire.ValidationDelay,
		Digest:          wire.Digest,
	}
	if p.Plugin == "" {
		p.Plugin = wire.ID
	}
	return nil
}

// CustomCPUModel is a custom CPU model definition
// (GET/POST /cluster/qemu/custom-cpu-models, GET/PUT/DELETE
// /cluster/qemu/custom-cpu-models/{cputype}). The wire keys use the pin's
// hyphenated names; Hidden tolerates the 0/1 int encoding on decode.
type CustomCPUModel struct {
	Name          string `json:"cputype,omitempty"`
	ReportedModel string `json:"reported-model,omitempty"`
	Flags         string `json:"flags,omitempty"`
	GuestPhysBits *int64 `json:"guest-phys-bits,omitempty"`
	Hidden        *bool  `json:"-"`
	HVVendorID    string `json:"hv-vendor-id,omitempty"`
	Level         *int64 `json:"level,omitempty"`
	PhysBits      string `json:"phys-bits,omitempty"`
	Digest        string `json:"-"`
}

// customCPUModelRaw mirrors the wire shape of CustomCPUModel with the
// lenient fields as json.RawMessage.
type customCPUModelRaw struct {
	Name          string          `json:"cputype"`
	ReportedModel string          `json:"reported-model"`
	Flags         string          `json:"flags"`
	GuestPhysBits *int64          `json:"guest-phys-bits"`
	Hidden        json.RawMessage `json:"hidden"`
	HVVendorID    string          `json:"hv-vendor-id"`
	Level         *int64          `json:"level"`
	PhysBits      string          `json:"phys-bits"`
	Digest        string          `json:"digest"`
}

// MarshalJSON encodes the model with the hidden flag as a plain bool.
func (m CustomCPUModel) MarshalJSON() ([]byte, error) {
	raw := customCPUModelRaw{
		Name:          m.Name,
		ReportedModel: m.ReportedModel,
		Flags:         m.Flags,
		GuestPhysBits: m.GuestPhysBits,
		HVVendorID:    m.HVVendorID,
		Level:         m.Level,
		PhysBits:      m.PhysBits,
	}
	if m.Hidden != nil {
		raw.Hidden = haBoolRaw(*m.Hidden)
	}
	return json.Marshal(raw)
}

// UnmarshalJSON decodes the boolish hidden flag.
func (m *CustomCPUModel) UnmarshalJSON(data []byte) error {
	var raw customCPUModelRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = CustomCPUModel{
		Name:          raw.Name,
		ReportedModel: raw.ReportedModel,
		Flags:         raw.Flags,
		GuestPhysBits: raw.GuestPhysBits,
		Hidden:        decodeBoolishPtr(raw.Hidden),
		HVVendorID:    raw.HVVendorID,
		Level:         raw.Level,
		PhysBits:      raw.PhysBits,
		Digest:        raw.Digest,
	}
	return nil
}

// RegisterAcmeAccount POSTs /cluster/acme/account and returns the UPID of
// the async registration task (the pin declares the return as string).
func (c *Client) RegisterAcmeAccount(ctx context.Context, reg AcmeAccountRegistration) (string, error) {
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/acme/account", reg, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// GetAcmeAccount reads /cluster/acme/account/{name}.
func (c *Client) GetAcmeAccount(ctx context.Context, name string) (*AcmeAccount, error) {
	var out AcmeAccount
	path := fmt.Sprintf("/cluster/acme/account/%s", name)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAcmeAccount PUTs /cluster/acme/account/{name} with a new contact
// list and returns the UPID of the async refresh task.
func (c *Client) UpdateAcmeAccount(ctx context.Context, name string, contact []string) (string, error) {
	var upid string
	body := map[string]any{}
	if len(contact) > 0 {
		body["contact"] = strings.Join(contact, ",")
	}
	path := fmt.Sprintf("/cluster/acme/account/%s", name)
	if err := c.Do(ctx, "PUT", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeactivateAcmeAccount DELETEs /cluster/acme/account/{name} (deactivates
// the account at the CA) and returns the UPID of the async task.
func (c *Client) DeactivateAcmeAccount(ctx context.Context, name string) (string, error) {
	var upid string
	path := fmt.Sprintf("/cluster/acme/account/%s", name)
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// ListAcmePlugins returns the plugins from GET /cluster/acme/plugins.
// pluginType optionally filters the list (dns | standalone).
func (c *Client) ListAcmePlugins(ctx context.Context, pluginType string) ([]AcmePlugin, error) {
	var out []AcmePlugin
	path := "/cluster/acme/plugins"
	if pluginType != "" {
		path += "?type=" + pluginType
	}
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAcmePlugin POSTs /cluster/acme/plugins with the supplied plugin.
func (c *Client) CreateAcmePlugin(ctx context.Context, plugin AcmePlugin) error {
	return c.Do(ctx, "POST", "/cluster/acme/plugins", plugin, nil)
}

// GetAcmePlugin reads /cluster/acme/plugins/{id}.
func (c *Client) GetAcmePlugin(ctx context.Context, id string) (*AcmePlugin, error) {
	var out AcmePlugin
	path := fmt.Sprintf("/cluster/acme/plugins/%s", id)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAcmePlugin PUTs /cluster/acme/plugins/{id} with the supplied body
// and translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateAcmePlugin(ctx context.Context, id string, plugin AcmePlugin, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/acme/plugins/%s", id), deleteFields)
	return c.Do(ctx, "PUT", path, plugin, nil)
}

// DeleteAcmePlugin DELETEs /cluster/acme/plugins/{id}.
func (c *Client) DeleteAcmePlugin(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/acme/plugins/%s", id), nil, nil)
}

// ListCustomCPUModels returns the models from GET
// /cluster/qemu/custom-cpu-models.
func (c *Client) ListCustomCPUModels(ctx context.Context) ([]CustomCPUModel, error) {
	var out []CustomCPUModel
	if err := c.Do(ctx, "GET", "/cluster/qemu/custom-cpu-models", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateCustomCPUModel POSTs /cluster/qemu/custom-cpu-models with the
// supplied model.
func (c *Client) CreateCustomCPUModel(ctx context.Context, model CustomCPUModel) error {
	return c.Do(ctx, "POST", "/cluster/qemu/custom-cpu-models", model, nil)
}

// GetCustomCPUModel reads /cluster/qemu/custom-cpu-models/{cputype}. The
// pin states the 'custom-' prefix on the name is optional.
func (c *Client) GetCustomCPUModel(ctx context.Context, name string) (*CustomCPUModel, error) {
	var out CustomCPUModel
	path := fmt.Sprintf("/cluster/qemu/custom-cpu-models/%s", name)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCustomCPUModel PUTs /cluster/qemu/custom-cpu-models/{cputype} with
// the supplied body and translates the delete slice into PVE's `delete`
// query parameter.
func (c *Client) UpdateCustomCPUModel(ctx context.Context, name string, model CustomCPUModel, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/qemu/custom-cpu-models/%s", name), deleteFields)
	return c.Do(ctx, "PUT", path, model, nil)
}

// DeleteCustomCPUModel DELETEs /cluster/qemu/custom-cpu-models/{cputype}.
func (c *Client) DeleteCustomCPUModel(ctx context.Context, name string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/qemu/custom-cpu-models/%s", name), nil, nil)
}
