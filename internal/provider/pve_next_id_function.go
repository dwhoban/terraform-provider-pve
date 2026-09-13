// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure the framework interface is satisfied.
var _ function.Function = &pveNextIdFunction{}

// pveNextIdClientSlot holds the provider-configured client for the
// pve_next_id function. framework v1.19 has no configure hook for
// functions, so PveProvider.Configure must publish the built client here
// (one line: pveNextIdClientSlot.Store(client)) right after client
// construction; until that line lands, the function reports a clear
// not-configured error.
var pveNextIdClientSlot atomic.Pointer[pveclient.Client]

// NewPveNextIdFunction returns the pve_next_id function implementation.
func NewPveNextIdFunction() function.Function {
	return &pveNextIdFunction{}
}

// pveNextIdFunction returns the next free VM ID (GET /cluster/nextid).
type pveNextIdFunction struct {
	// client overrides pveNextIdClientSlot when set (used by tests).
	client *pveclient.Client
}

// Metadata implements function.Function.
func (f *pveNextIdFunction) Metadata(_ context.Context, _ function.MetadataRequest, resp *function.MetadataResponse) {
	// Provider functions are invoked as provider::pve::<name>; the provider
	// name itself is not part of the function name.
	resp.Name = TypeNamePveNextId
}

// Definition implements function.Function.
func (f *pveNextIdFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary:             "Return the next free VM ID in the cluster.",
		MarkdownDescription: "Returns the next free VM ID as reported by `GET /cluster/nextid`, as a string. The ID is only free at the time of the check; allocating it requires creating a guest or pool with that ID.",
		Return:              function.StringReturn{},
	}
}

// Run implements function.Function.
func (f *pveNextIdFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	client := f.client
	if client == nil {
		client = pveNextIdClientSlot.Load()
	}
	if client == nil {
		resp.Error = function.NewFuncError("pve_next_id has no configured Proxmox VE client: the provider is not configured in this context")
		return
	}
	id, err := client.GetNextID(ctx)
	if err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("reading next free VM ID from %s failed: %s", client.Endpoint(), err))
		return
	}
	resp.Error = resp.Result.Set(ctx, types.StringValue(strconv.FormatInt(id, 10)))
}
