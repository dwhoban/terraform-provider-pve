// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// This file is the scaffolding every generic engine shares: the descriptor
// and client it was instantiated with, plus the one spelling for operation
// diagnostics ("Error Creating pve_vm" over "creating pve_vm (…): …").

// engineBase is embedded by the generic engines: it carries the descriptor
// the engine renders and the client the provider configured.
type engineBase struct {
	desc   *TypeDescriptor
	client *pveclient.Client
}

// setClient stores the provider-configured client. A nil ProviderData
// (provider not configured) leaves the engine unconfigured — requireClient
// reports that at use time — while anything non-nil that is not a
// *pveclient.Client is a programming error, diagnosed by the caller.
func (b *engineBase) setClient(providerData any) error {
	if providerData == nil {
		return nil
	}
	c, ok := providerData.(*pveclient.Client)
	if !ok {
		return fmt.Errorf("expected *pveclient.Client for %s, got: %T", b.desc.Name, providerData)
	}
	b.client = c
	return nil
}

// requireClient reports whether the engine can talk to the API.
func (b *engineBase) requireClient() error {
	if b.client == nil {
		//nolint:staticcheck // ST1005: the period is user-facing prose
		return fmt.Errorf("%s: provider client is not configured.", b.desc.Name)
	}
	return nil
}

// opError emits the standard operation diagnostic: summary
// "Error <Verb> <name>", detail "<verb> <name> (<ident>): <err>".
func opError(diags *diag.Diagnostics, verb string, d *TypeDescriptor, ident string, err error) {
	diags.AddError(
		fmt.Sprintf("Error %s %s", verb, d.Name),
		fmt.Sprintf("%s %s (%s): %s", strings.ToLower(verb), d.Name, ident, err),
	)
}
