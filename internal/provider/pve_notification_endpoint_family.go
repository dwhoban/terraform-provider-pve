// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Shared machinery for the four notification endpoint resources and data
// sources (sendmail, gotify, smtp, webhook). Every mutation is synchronous
// per the pin — no task is spawned.

// notificationEndpointConfigureResource extracts the shared client from
// provider data for notification endpoint resources. Nil provider data
// leaves the resource unconfigured (unit tests).
func notificationEndpointConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// notificationEndpointConfigureDataSource extracts the shared client from
// provider data for notification endpoint data sources. Nil provider data
// leaves the data source unconfigured (unit tests).
func notificationEndpointConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// notificationEndpointListToTF builds a string list attribute from the
// endpoint's recipients, mapping an absent list to null.
func notificationEndpointListToTF(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	return listStringToTF(in)
}

// notificationEndpointMapToTF builds a string map attribute from decoded
// webhook headers or secrets, mapping an absent map to null.
func notificationEndpointMapToTF(in map[string]string) types.Map {
	if len(in) == 0 {
		return types.MapNull(types.StringType)
	}
	elems := make(map[string]attr.Value, len(in))
	for k, v := range in {
		elems[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elems)
}

// notificationEndpointMapFromTF flattens a string map attribute into a Go
// map; null and empty maps both produce a nil map.
func notificationEndpointMapFromTF(in types.Map) map[string]string {
	if in.IsNull() || in.IsUnknown() || len(in.Elements()) == 0 {
		return nil
	}
	out := make(map[string]string, len(in.Elements()))
	for k, e := range in.Elements() {
		s, ok := e.(types.String)
		if !ok {
			continue
		}
		out[k] = s.ValueString()
	}
	return out
}

// notificationEndpointSendmailFromModel projects the sendmail resource
// model into the wire body.
func notificationEndpointSendmailFromModel(m pveNotificationEndpointSendmailResourceModel) pveclient.NotificationEndpoint {
	ep := pveclient.NotificationEndpoint{
		Type: pveclient.NotificationEndpointTypeSendmail,
		Name: m.Name.ValueString(),
	}
	if !m.MailTo.IsNull() && !m.MailTo.IsUnknown() {
		ep.MailTo = listStringFromTF(m.MailTo)
	}
	if !m.MailToUser.IsNull() && !m.MailToUser.IsUnknown() {
		ep.MailToUser = listStringFromTF(m.MailToUser)
	}
	if !m.FromAddress.IsNull() && !m.FromAddress.IsUnknown() {
		ep.FromAddress = m.FromAddress.ValueString()
	}
	if !m.Author.IsNull() && !m.Author.IsUnknown() {
		ep.Author = m.Author.ValueString()
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		ep.Comment = m.Comment.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		ep.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	return ep
}

// notificationEndpointSendmailDeleteFields returns the PVE field names to
// clear on update: optional attributes present in state but null in plan.
func notificationEndpointSendmailDeleteFields(plan, state pveNotificationEndpointSendmailResourceModel) []string {
	var out []string
	if plan.MailTo.IsNull() && !state.MailTo.IsNull() {
		out = append(out, "mailto")
	}
	if plan.MailToUser.IsNull() && !state.MailToUser.IsNull() {
		out = append(out, "mailto-user")
	}
	if plan.FromAddress.IsNull() && !state.FromAddress.IsNull() {
		out = append(out, "from-address")
	}
	if plan.Author.IsNull() && !state.Author.IsNull() {
		out = append(out, "author")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}

// notificationEndpointGotyFromModel projects the gotify resource model
// into the wire body. The token is write-only and is sent whenever set.
func notificationEndpointGotyFromModel(m pveNotificationEndpointGotyResourceModel) pveclient.NotificationEndpoint {
	ep := pveclient.NotificationEndpoint{
		Type:   pveclient.NotificationEndpointTypeGotify,
		Name:   m.Name.ValueString(),
		Server: m.Server.ValueString(),
		Token:  m.Token.ValueString(),
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		ep.Comment = m.Comment.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		ep.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	return ep
}

// notificationEndpointGotyDeleteFields returns the PVE field names to
// clear on update.
func notificationEndpointGotyDeleteFields(plan, state pveNotificationEndpointGotyResourceModel) []string {
	var out []string
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}

// notificationEndpointSMTPFromModel projects the smtp resource model into
// the wire body. The password is write-only and is sent whenever set.
func notificationEndpointSMTPFromModel(m pveNotificationEndpointSMTPResourceModel) pveclient.NotificationEndpoint {
	ep := pveclient.NotificationEndpoint{
		Type:        pveclient.NotificationEndpointTypeSMTP,
		Name:        m.Name.ValueString(),
		Server:      m.Server.ValueString(),
		FromAddress: m.FromAddress.ValueString(),
	}
	if !m.Username.IsNull() && !m.Username.IsUnknown() {
		ep.Username = m.Username.ValueString()
	}
	if !m.Password.IsNull() && !m.Password.IsUnknown() {
		ep.Password = m.Password.ValueString()
	}
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() {
		ep.Mode = m.Mode.ValueString()
	}
	if !m.Port.IsNull() && !m.Port.IsUnknown() {
		ep.Port = pveclient.HAInt64Ptr(m.Port.ValueInt64())
	}
	if !m.MailTo.IsNull() && !m.MailTo.IsUnknown() {
		ep.MailTo = listStringFromTF(m.MailTo)
	}
	if !m.MailToUser.IsNull() && !m.MailToUser.IsUnknown() {
		ep.MailToUser = listStringFromTF(m.MailToUser)
	}
	if !m.Author.IsNull() && !m.Author.IsUnknown() {
		ep.Author = m.Author.ValueString()
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		ep.Comment = m.Comment.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		ep.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	return ep
}

// notificationEndpointSMTPDeleteFields returns the PVE field names to clear
// on update: optional attributes present in state but null in plan. The
// password is clearable too (PVE removes it from the endpoint).
func notificationEndpointSMTPDeleteFields(plan, state pveNotificationEndpointSMTPResourceModel) []string {
	var out []string
	if plan.Username.IsNull() && !state.Username.IsNull() {
		out = append(out, "username")
	}
	if plan.Password.IsNull() && !state.Password.IsNull() {
		out = append(out, "password")
	}
	if plan.Mode.IsNull() && !state.Mode.IsNull() {
		out = append(out, "mode")
	}
	if plan.Port.IsNull() && !state.Port.IsNull() {
		out = append(out, "port")
	}
	if plan.MailTo.IsNull() && !state.MailTo.IsNull() {
		out = append(out, "mailto")
	}
	if plan.MailToUser.IsNull() && !state.MailToUser.IsNull() {
		out = append(out, "mailto-user")
	}
	if plan.Author.IsNull() && !state.Author.IsNull() {
		out = append(out, "author")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}

// notificationEndpointWebhookFromModel projects the webhook resource model
// into the wire body; headers and secret values are base64-encoded by the
// client.
func notificationEndpointWebhookFromModel(m pveNotificationEndpointWebhookResourceModel) pveclient.NotificationEndpoint {
	ep := pveclient.NotificationEndpoint{
		Type:   pveclient.NotificationEndpointTypeWebhook,
		Name:   m.Name.ValueString(),
		URL:    m.URL.ValueString(),
		Method: m.Method.ValueString(),
	}
	if !m.Body.IsNull() && !m.Body.IsUnknown() {
		ep.Body = m.Body.ValueString()
	}
	if !m.Headers.IsNull() && !m.Headers.IsUnknown() {
		ep.Headers = notificationEndpointMapFromTF(m.Headers)
	}
	if !m.Secrets.IsNull() && !m.Secrets.IsUnknown() {
		ep.Secrets = notificationEndpointMapFromTF(m.Secrets)
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		ep.Comment = m.Comment.ValueString()
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		ep.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	return ep
}

// notificationEndpointWebhookDeleteFields returns the PVE field names to
// clear on update: optional attributes present in state but null in plan.
func notificationEndpointWebhookDeleteFields(plan, state pveNotificationEndpointWebhookResourceModel) []string {
	var out []string
	if plan.Body.IsNull() && !state.Body.IsNull() {
		out = append(out, "body")
	}
	if plan.Headers.IsNull() && !state.Headers.IsNull() {
		out = append(out, "header")
	}
	if plan.Secrets.IsNull() && !state.Secrets.IsNull() {
		out = append(out, "secret")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}
