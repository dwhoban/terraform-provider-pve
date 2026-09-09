// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// pveVmDiskModel is one entry of the pve_vm disks list.
type pveVmDiskModel struct {
	ID       types.String `tfsdk:"id"`
	Storage  types.String `tfsdk:"storage"`
	Size     types.String `tfsdk:"size"`
	Format   types.String `tfsdk:"format"`
	Cache    types.String `tfsdk:"cache"`
	Discard  types.String `tfsdk:"discard"`
	IOThread types.Bool   `tfsdk:"iothread"`
	SSD      types.Bool   `tfsdk:"ssd"`
}

// pveVmNetModel is one entry of the pve_vm network_interfaces list.
type pveVmNetModel struct {
	ID       types.String `tfsdk:"id"`
	Model    types.String `tfsdk:"model"`
	Bridge   types.String `tfsdk:"bridge"`
	VlanTag  types.Int64  `tfsdk:"vlan_tag"`
	Firewall types.Bool   `tfsdk:"firewall"`
	MACAddr  types.String `tfsdk:"macaddr"`
	Queues   types.Int64  `tfsdk:"queues"`
}

// pveVmCloudInitModel is the pve_vm cloud_init block. Password is
// write-only: PVE never returns cipassword, so reads leave it null.
type pveVmCloudInitModel struct {
	User         types.String      `tfsdk:"user"`
	Password     types.String      `tfsdk:"password"`
	SearchDomain types.String      `tfsdk:"searchdomain"`
	Nameserver   types.String      `tfsdk:"nameserver"`
	SSHKeys      types.String      `tfsdk:"sshkeys"`
	Ipconfig     map[string]string `tfsdk:"ipconfig"`
}

// pveVmCloneModel is the create-only pve_vm clone block.
type pveVmCloneModel struct {
	SourceVmid types.Int64  `tfsdk:"source_vmid"`
	Name       types.String `tfsdk:"name"`
	Full       types.Bool   `tfsdk:"full"`
	Storage    types.String `tfsdk:"storage"`
	Format     types.String `tfsdk:"format"`
	Pool       types.String `tfsdk:"pool"`
}

// vmDiskKeyRe matches modeled drive config keys (ide/sata/scsi/virtio
// indexes and efidisk0). It backs both the schema id validator and the
// config scan on read.
var vmDiskKeyRe = regexp.MustCompile(`^(virtio|sata|scsi|ide)[0-9]+$|^efidisk0$`)

// vmNetKeyRe matches network device config keys.
var vmNetKeyRe = regexp.MustCompile(`^net[0-9]+$`)

// vmIpconfigKeyRe matches cloud-init ipconfig entries.
var vmIpconfigKeyRe = regexp.MustCompile(`^ipconfig[0-9]+$`)

// vmParseImportID splits a pve_vm import ID of the form `<node>/<vmid>`.
func vmParseImportID(id string) (string, int64, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", 0, fmt.Errorf("expected import ID format <node>/<vmid>, got %q", id)
	}
	vmid, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("vmid %q in import ID %q is not an integer: %w", parts[1], id, err)
	}
	return parts[0], vmid, nil
}

// vmDiskSizeBytes parses a PVE disk size ("32G", "512M", "0.5T", plain
// bytes) into bytes.
func vmDiskSizeBytes(size string) (int64, error) {
	trimmed := strings.TrimSpace(size)
	if trimmed == "" {
		return 0, fmt.Errorf("empty disk size")
	}
	units := []struct {
		suffix string
		factor int64
	}{
		{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(trimmed, unit.suffix) && len(trimmed) > len(unit.suffix) {
			trimmed = strings.TrimSuffix(trimmed, unit.suffix)
			return vmSizeFromFloat(trimmed, unit.factor)
		}
	}
	return vmSizeFromFloat(trimmed, 1)
}

// vmSizeFromFloat parses the numeric part of a size and scales it.
func vmSizeFromFloat(number string, factor int64) (int64, error) {
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid disk size %q: %w", number, err)
	}
	return int64(value * float64(factor)), nil
}

// vmFormatSizeBytes renders a byte count the way PVE reports drive sizes,
// using the largest unit that divides the value evenly.
func vmFormatSizeBytes(bytes int64) string {
	units := []struct {
		suffix string
		size   int64
	}{
		{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10},
	}
	for _, unit := range units {
		if bytes >= unit.size && bytes%unit.size == 0 {
			return strconv.FormatInt(bytes/unit.size, 10) + unit.suffix
		}
	}
	return strconv.FormatInt(bytes, 10)
}

// vmDriveLive is the parsed state of one PVE drive value.
type vmDriveLive struct {
	// Volume is the backing volume reference (storage:volume or path).
	Volume string
	// Storage is the storage part of Volume (empty for bare paths).
	Storage  string
	Size     string
	Cache    string
	Discard  string
	Format   string
	IOThread bool
	SSD      bool
}

// vmParseDriveValue parses a PVE drive value such as
// "local-lvm:vm-100-disk-0,size=32G,iothread=1" or "none,media=cdrom".
func vmParseDriveValue(value string) vmDriveLive {
	var live vmDriveLive
	for i, raw := range strings.Split(value, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		key, val, hasKey := strings.Cut(token, "=")
		switch {
		case !hasKey && i == 0:
			live.Volume = token
			if colon := strings.Index(token, ":"); colon >= 0 {
				live.Storage = token[:colon]
			}
		case key == "size":
			live.Size = val
		case key == "cache":
			live.Cache = val
		case key == "discard":
			live.Discard = val
		case key == "format":
			live.Format = val
		case key == "iothread":
			live.IOThread = val == "1" || val == "true" || val == "on"
		case key == "ssd":
			live.SSD = val == "1" || val == "true" || val == "on"
		}
	}
	return live
}

// vmRenderNewDrive renders a drive that allocates a fresh volume, in the
// PVE "STORAGE_ID:SIZE_IN_GiB[,opts]" syntax. efidisk0 ignores the size
// per the pin and allocates on the storage alone.
func vmRenderNewDrive(m pveVmDiskModel) (string, error) {
	driveID := m.ID.ValueString()
	storage := m.Storage.ValueString()
	if storage == "" {
		return "", fmt.Errorf("disk %s requires storage to allocate a new volume", driveID)
	}
	if driveID == "efidisk0" {
		return storage + ":1", nil
	}
	size := m.Size.ValueString()
	if size == "" {
		return "", fmt.Errorf("disk %s requires size to allocate a new volume (PVE allocates whole GiB)", driveID)
	}
	total, err := vmDiskSizeBytes(size)
	if err != nil {
		return "", fmt.Errorf("disk %s: %w", driveID, err)
	}
	gib := (total + (1 << 30) - 1) >> 30
	if gib < 1 {
		gib = 1
	}
	parts := []string{fmt.Sprintf("%s:%d", storage, gib)}
	if !m.Format.IsNull() {
		parts = append(parts, "format="+m.Format.ValueString())
	}
	if !m.Cache.IsNull() {
		parts = append(parts, "cache="+m.Cache.ValueString())
	}
	if !m.Discard.IsNull() {
		parts = append(parts, "discard="+m.Discard.ValueString())
	}
	if !m.IOThread.IsNull() {
		parts = append(parts, "iothread="+qemuVMFlagLocal(m.IOThread.ValueBool()))
	}
	if !m.SSD.IsNull() {
		parts = append(parts, "ssd="+qemuVMFlagLocal(m.SSD.ValueBool()))
	}
	return strings.Join(parts, ","), nil
}

// vmRenderExistingDrive re-renders an existing drive around its current
// volume, applying the modeled properties.
func vmRenderExistingDrive(volume string, m pveVmDiskModel) string {
	parts := []string{volume}
	if !m.Size.IsNull() && m.Size.ValueString() != "" {
		parts = append(parts, "size="+m.Size.ValueString())
	}
	if !m.Cache.IsNull() {
		parts = append(parts, "cache="+m.Cache.ValueString())
	}
	if !m.Discard.IsNull() {
		parts = append(parts, "discard="+m.Discard.ValueString())
	}
	if !m.IOThread.IsNull() {
		parts = append(parts, "iothread="+qemuVMFlagLocal(m.IOThread.ValueBool()))
	}
	if !m.SSD.IsNull() {
		parts = append(parts, "ssd="+qemuVMFlagLocal(m.SSD.ValueBool()))
	}
	return strings.Join(parts, ",")
}

// qemuVMFlagLocal renders a boolean as the PVE 1/0 flag.
func qemuVMFlagLocal(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// vmRenderNet renders one network device in PVE config syntax, e.g.
// "virtio=BC:24:11:2F:4E:8D,bridge=vmbr0,firewall=1". A null MAC renders
// the bare model so PVE keeps its generated address untouched on create.
func vmRenderNet(m pveVmNetModel) string {
	parts := []string{m.Model.ValueString()}
	if !m.Bridge.IsNull() {
		parts = append(parts, "bridge="+m.Bridge.ValueString())
	}
	if !m.VlanTag.IsNull() {
		parts = append(parts, "tag="+strconv.FormatInt(m.VlanTag.ValueInt64(), 10))
	}
	if !m.Firewall.IsNull() {
		parts = append(parts, "firewall="+qemuVMFlagLocal(m.Firewall.ValueBool()))
	}
	if !m.Queues.IsNull() {
		parts = append(parts, "queues="+strconv.FormatInt(m.Queues.ValueInt64(), 10))
	}
	if !m.MACAddr.IsNull() && m.MACAddr.ValueString() != "" {
		parts = append(parts, "macaddr="+m.MACAddr.ValueString())
	}
	return strings.Join(parts, ",")
}

// vmNetLive is the parsed state of one PVE network device value.
type vmNetLive struct {
	Model    string
	Bridge   string
	MACAddr  string
	Queues   string
	Tag      string
	Firewall *bool
}

// vmParseNetValue parses a PVE network device value such as
// "virtio=BC:24:11:2F:4E:8D,bridge=vmbr0,tag=10,firewall=1".
func vmParseNetValue(value string) vmNetLive {
	var live vmNetLive
	for i, raw := range strings.Split(value, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		key, val, hasKey := strings.Cut(token, "=")
		if !hasKey && i == 0 {
			live.Model = token
			continue
		}
		if !hasKey {
			continue
		}
		switch key {
		case "model":
			live.Model = val
		case "bridge":
			live.Bridge = val
		case "macaddr":
			live.MACAddr = val
		case "queues":
			live.Queues = val
		case "tag":
			live.Tag = val
		case "firewall":
			enabled := val == "1" || val == "true"
			live.Firewall = &enabled
		default:
			// The pin's default-key form puts the model name as the key
			// and the MAC as the value (e.g. "virtio=BC:24:11:...").
			if i == 0 && strings.Count(val, ":") == 5 {
				live.Model = key
				live.MACAddr = val
			}
		}
	}
	return live
}

// vmRawString decodes an optional config value into a string: JSON
// strings are unquoted, other scalars keep their literal token text.
func vmRawString(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return &text
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		literal := strconv.FormatFloat(number, 'f', -1, 64)
		return &literal
	}
	literal := string(raw)
	return &literal
}

// vmRawInt64 decodes an optional config value into an integer, tolerating
// JSON numbers and numeric strings.
func vmRawInt64(raw json.RawMessage) *int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		value := int64(number)
		return &value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64); err == nil {
			return &value
		}
	}
	return nil
}

// vmRawBool decodes an optional config value into a boolean, tolerating
// JSON booleans, 0/1 integers, and "1"/"0"/"true"/"false" strings.
func vmRawBool(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var flag bool
	if err := json.Unmarshal(raw, &flag); err == nil {
		return &flag
	}
	switch text := vmRawString(raw); {
	case text == nil:
		return nil
	case *text == "1" || *text == "true" || *text == "on":
		value := true
		return &value
	case *text == "0" || *text == "false" || *text == "off":
		value := false
		return &value
	}
	return nil
}

// vmAgentBool decodes the agent config key, which may be the legacy
// "1"/"0" string or the pin's "enabled=<1|0>,..." compound form.
func vmAgentBool(raw json.RawMessage) *bool {
	if flag := vmRawBool(raw); flag != nil {
		return flag
	}
	text := vmRawString(raw)
	if text == nil {
		return nil
	}
	for _, token := range strings.Split(*text, ",") {
		if key, val, ok := strings.Cut(strings.TrimSpace(token), "="); ok && key == "enabled" {
			enabled := val == "1" || val == "true"
			return &enabled
		}
	}
	return nil
}

// vmCPUTypeFromRaw extracts the cputype from the pin's cpu config value,
// which may be a bare type ("kvm64") or a compound ("cputype=host,...").
func vmCPUTypeFromRaw(raw json.RawMessage) *string {
	text := vmRawString(raw)
	if text == nil || *text == "" {
		return nil
	}
	first := strings.SplitN(*text, ",", 2)[0]
	if _, val, ok := strings.Cut(first, "="); ok {
		return &val
	}
	return &first
}

// vmBootOrderFromRaw extracts the ordered device list from the boot
// config value ("order=scsi0;net0").
func vmBootOrderFromRaw(raw json.RawMessage) []string {
	text := vmRawString(raw)
	if text == nil {
		return nil
	}
	order := *text
	if _, val, ok := strings.Cut(order, "order="); ok {
		order = val
	}
	if order == "" {
		return nil
	}
	return strings.Split(order, ";")
}

// vmTagsFromRaw splits the pin's pve-tag-list string into tags.
func vmTagsFromRaw(raw json.RawMessage) []string {
	text := vmRawString(raw)
	if text == nil || *text == "" {
		return nil
	}
	tags := []string{}
	for _, tag := range strings.FieldsFunc(*text, func(r rune) bool { return r == ';' || r == ',' }) {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// vmSortDriveKeys orders drive config keys naturally (ide, sata, scsi,
// virtio, then efidisk0, each by index).
func vmSortDriveKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		aPrefix, aIndex := vmDriveKeyParts(keys[i])
		bPrefix, bIndex := vmDriveKeyParts(keys[j])
		if aPrefix != bPrefix {
			return aPrefix < bPrefix
		}
		return aIndex < bIndex
	})
}

// vmDriveKeyParts splits a drive key into its bus prefix and index.
func vmDriveKeyParts(key string) (string, int) {
	digits := len(key)
	for digits > 0 && key[digits-1] >= '0' && key[digits-1] <= '9' {
		digits--
	}
	index, _ := strconv.Atoi(key[digits:])
	return key[:digits], index
}

// vmDisksFromConfig scans the config map for drive keys and parses each
// value into a pveVmDiskModel.
func vmDisksFromConfig(config map[string]json.RawMessage) []pveVmDiskModel {
	keys := []string{}
	for key := range config {
		if vmDiskKeyRe.MatchString(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	vmSortDriveKeys(keys)
	disks := make([]pveVmDiskModel, 0, len(keys))
	for _, key := range keys {
		value := vmRawString(config[key])
		if value == nil {
			continue
		}
		live := vmParseDriveValue(*value)
		disks = append(disks, pveVmDiskModel{
			ID:       types.StringValue(key),
			Storage:  vmStringPtrToTF(live.Storage),
			Size:     vmStringPtrToTF(live.Size),
			Format:   vmStringPtrToTF(live.Format),
			Cache:    vmStringPtrToTF(live.Cache),
			Discard:  vmStringPtrToTF(live.Discard),
			IOThread: types.BoolValue(live.IOThread),
			SSD:      types.BoolValue(live.SSD),
		})
	}
	return disks
}

// vmNetsFromConfig scans the config map for network keys and parses each
// value into a pveVmNetModel.
func vmNetsFromConfig(config map[string]json.RawMessage) []pveVmNetModel {
	keys := []string{}
	for key := range config {
		if vmNetKeyRe.MatchString(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	nets := make([]pveVmNetModel, 0, len(keys))
	for _, key := range keys {
		value := vmRawString(config[key])
		if value == nil {
			continue
		}
		live := vmParseNetValue(*value)
		nets = append(nets, pveVmNetModel{
			ID:       types.StringValue(key),
			Model:    vmStringPtrToTF(live.Model),
			Bridge:   vmStringPtrToTF(live.Bridge),
			VlanTag:  vmAtoiToTF(live.Tag),
			Firewall: vmBoolPtrToTF(live.Firewall),
			MACAddr:  vmStringPtrToTF(live.MACAddr),
			Queues:   vmAtoiToTF(live.Queues),
		})
	}
	return nets
}

// vmCloudInitFromConfig builds the cloud_init view from the config map.
// Password is never populated (write-only).
func vmCloudInitFromConfig(config map[string]json.RawMessage) *pveVmCloudInitModel {
	ipconfigs := map[string]string{}
	for key := range config {
		if vmIpconfigKeyRe.MatchString(key) {
			if value := vmRawString(config[key]); value != nil {
				ipconfigs[key] = *value
			}
		}
	}
	model := &pveVmCloudInitModel{
		User:         vmTFStringFromRaw(config["ciuser"]),
		Password:     types.StringNull(),
		SearchDomain: vmTFStringFromRaw(config["searchdomain"]),
		Nameserver:   vmTFStringFromRaw(config["nameserver"]),
		SSHKeys:      vmTFDecodedStringFromRaw(config["sshkeys"]),
	}
	if len(ipconfigs) > 0 {
		model.Ipconfig = ipconfigs
	}
	if model.User.IsNull() && model.SearchDomain.IsNull() && model.Nameserver.IsNull() &&
		model.SSHKeys.IsNull() && len(ipconfigs) == 0 {
		return nil
	}
	return model
}

// vmTFStringFromRaw renders an optional config value as a null-able
// types.String.
func vmTFStringFromRaw(raw json.RawMessage) types.String {
	if value := vmRawString(raw); value != nil {
		return types.StringValue(*value)
	}
	return types.StringNull()
}

// vmTFDecodedStringFromRaw renders a percent-encoded config value
// (sshkeys) after decoding it back to plain text.
func vmTFDecodedStringFromRaw(raw json.RawMessage) types.String {
	value := vmRawString(raw)
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(vmDecodePercent(*value))
}

// vmDecodePercent reverses PVE's urlencoded values (sshkeys), falling
// back to the raw input when the stored value is not valid encoding.
func vmDecodePercent(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

// vmTFIntFromRaw renders an optional config value as types.Int64.
func vmTFIntFromRaw(raw json.RawMessage) types.Int64 {
	if value := vmRawInt64(raw); value != nil {
		return types.Int64Value(*value)
	}
	return types.Int64Null()
}

// vmTFBoolFromRaw renders an optional config value as types.Bool.
func vmTFBoolFromRaw(raw json.RawMessage) types.Bool {
	if value := vmRawBool(raw); value != nil {
		return types.BoolValue(*value)
	}
	return types.BoolNull()
}

// vmStringPtrToTF converts a string pointer to types.String.
func vmStringPtrToTF(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

// vmBoolPtrToTF converts a bool pointer to types.Bool.
func vmBoolPtrToTF(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}

// vmAtoiToTF parses a numeric string into types.Int64 (null when empty or
// unparsable).
func vmAtoiToTF(value string) types.Int64 {
	if value == "" {
		return types.Int64Null()
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return types.Int64Null()
	}
	return types.Int64Value(number)
}

// vmMergePending applies staged pending changes onto the config map so
// the computed view surfaces drift, and returns the pending keys. A
// pending delete row removes the key from the map entirely.
func vmMergePending(config map[string]json.RawMessage, pending []pveclient.QemuVMPendingChange) []string {
	pendingKeys := []string{}
	for _, change := range pending {
		if change.Delete != nil && *change.Delete > 0 {
			delete(config, change.Key)
			pendingKeys = append(pendingKeys, change.Key)
			continue
		}
		if change.Pending != nil {
			config[change.Key] = vmJSONString(*change.Pending)
			pendingKeys = append(pendingKeys, change.Key)
		}
	}
	if len(pendingKeys) == 0 {
		return nil
	}
	return pendingKeys
}

// vmJSONString encodes a plain string as a JSON string literal.
func vmJSONString(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return encoded
}

// vmRenderPlanDrives renders every planned drive against the live
// config: new drives allocate volumes, existing drives re-render around
// their current backing volume when the modeled properties changed.
func vmRenderPlanDrives(planDisks []pveVmDiskModel, live map[string]json.RawMessage) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	sets := map[string]string{}
	for _, disk := range planDisks {
		driveID := disk.ID.ValueString()
		raw, exists := live[driveID]
		if !exists {
			rendered, err := vmRenderNewDrive(disk)
			if err != nil {
				diags.AddError("Invalid disk configuration", err.Error())
				continue
			}
			sets[driveID] = rendered
			continue
		}
		stored := vmRawString(raw)
		if stored == nil {
			continue
		}
		proposed := vmRenderExistingDrive(vmParseDriveValue(*stored).Volume, disk)
		if proposed != *stored {
			sets[driveID] = proposed
		}
	}
	return sets, diags
}

// vmEnsureStopped brings a running VM down: ACPI shutdown first (with the
// caller's timeout), falling back to a hard stop when the shutdown task
// fails or the VM keeps running.
func vmEnsureStopped(ctx context.Context, client *pveclient.Client, node string, vmid int64, timeout *int64) error {
	status, err := client.GetQemuVMStatusCurrentMinimal(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("read qemu VM %d status before stop on node %s: %w", vmid, node, err)
	}
	if status.Status != "running" {
		return nil
	}
	upid, err := client.QemuVMShutdown(ctx, node, vmid, timeout, nil)
	if err != nil {
		return fmt.Errorf("shutdown qemu VM %d on node %s: %w", vmid, node, err)
	}
	if _, err := client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err == nil {
		return nil
	}
	// ACPI shutdown failed or timed out; pull the plug instead.
	upid, err = client.QemuVMStop(ctx, node, vmid, timeout)
	if err != nil {
		return fmt.Errorf("stop qemu VM %d on node %s after failed shutdown: %w", vmid, node, err)
	}
	if _, err := client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return fmt.Errorf("wait for qemu VM %d stop on node %s: %w", vmid, node, err)
	}
	return nil
}

// vmAgentBoolToTF renders the agent config key as types.Bool.
func vmAgentBoolToTF(raw json.RawMessage) types.Bool {
	if flag := vmAgentBool(raw); flag != nil {
		return types.BoolValue(*flag)
	}
	return types.BoolNull()
}

// vmCPUTypeToTF renders the pin's cpu config value as the modeled
// cpu_type string.
func vmCPUTypeToTF(raw json.RawMessage) types.String {
	if value := vmCPUTypeFromRaw(raw); value != nil {
		return types.StringValue(*value)
	}
	return types.StringNull()
}

// vmConfigIntoResourceModel fills the modeled config view of the pve_vm
// resource model from the raw config and status reads. Fields the API
// does not return (clone, stop_on_destroy, migrate options, cloud-init
// password) are left untouched.
func vmConfigIntoResourceModel(config map[string]json.RawMessage, status *pveclient.QemuVMStatus, m *pveVmResourceModel) {
	m.Name = vmTFStringFromRaw(config["name"])
	m.Description = vmTFStringFromRaw(config["description"])
	m.Tags = listStringToTF(vmTagsFromRaw(config["tags"]))
	m.Onboot = vmTFBoolFromRaw(config["onboot"])
	m.Protection = vmTFBoolFromRaw(config["protection"])
	m.Template = vmTFBoolFromRaw(config["template"])
	m.Agent = vmAgentBoolToTF(config["agent"])
	m.BIOS = vmTFStringFromRaw(config["bios"])
	m.Machine = vmTFStringFromRaw(config["machine"])
	m.OSType = vmTFStringFromRaw(config["ostype"])
	m.CPUType = vmCPUTypeToTF(config["cpu"])
	m.SCSIHW = vmTFStringFromRaw(config["scsihw"])
	m.Cores = vmTFIntFromRaw(config["cores"])
	m.Sockets = vmTFIntFromRaw(config["sockets"])
	m.Memory = vmTFIntFromRaw(config["memory"])
	m.BootOrder = listStringToTF(vmBootOrderFromRaw(config["boot"]))
	m.Disks = vmDisksFromConfig(config)
	m.NetworkInterfaces = vmNetsFromConfig(config)
	m.CloudInit = vmCloudInitFromConfig(config)
	if status != nil {
		m.Status = types.StringValue(status.Status)
		m.Uptime = types.Int64Value(status.Uptime)
		m.MaxMem = types.Int64Value(status.MaxMem)
		m.MaxCPU = types.Int64Value(status.MaxCPU)
		m.Started = types.BoolValue(status.Status == "running")
		if m.Template.IsNull() {
			m.Template = types.BoolValue(status.Template)
		}
	}
}

// vmDiagsText renders all error diagnostics as one flat string for
// wrapped error returns.
func vmDiagsText(diags diag.Diagnostics) string {
	var builder strings.Builder
	for _, err := range diags.Errors() {
		builder.WriteString(err.Summary())
		builder.WriteString(": ")
		builder.WriteString(err.Detail())
		builder.WriteString("; ")
	}
	return builder.String()
}
