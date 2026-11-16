// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// The apply pipeline carries a managed resource's Create or Update from plan
// to final state: mutation, opt-in apply of pending changes, read-back, ID finalization. The
// framework-facing methods only extract values, call apply once, and set the
// state from the (mutated) values map.

// applyVerb is the per-verb policy of the apply pipeline.
type applyVerb struct {
	creating         bool
	gerund           string // diagnostic summary verb: "Error <gerund> <type>"
	noun             string // read-back summary: "Error Reading <type> After <noun>"
	tolerateVanished bool   // a read-back that finds nothing (or 404) is not an error
	ident            func(*TypeDescriptor, map[string]attr.Value) string
}

var (
	// applyCreate fails when the new object cannot be read back.
	applyCreate = applyVerb{creating: true, gerund: "Creating", noun: "Create", ident: describeValues}
	// applyUpdate tolerates a vanished read-back: state is kept from the plan
	// values and the next refresh decides whether the object is gone.
	applyUpdate = applyVerb{gerund: "Updating", noun: "Update", tolerateVanished: true, ident: describeID}
)

// apply runs the verb against the API. values holds the plan values and is
// updated in place to the final state values; state is the prior state
// (Update only). The client must already be required by the caller.
func (r *genericResource) apply(ctx context.Context, v applyVerb, values, state map[string]attr.Value) diag.Diagnostics {
	var diags diag.Diagnostics
	d := r.desc

	var mutated *Op
	if v.creating {
		mutated = d.Create
		payload, waited, err := r.mutateCreate(ctx, values)
		if err != nil {
			opError(&diags, v.gerund, d, v.ident(d, values), err)
			return diags
		}
		r.fillAllocatedID(payload, waited, values)
		if s, ok := payloadString(payload); ok && d.Create != nil && d.Create.ResultAttr != "" {
			if a, found := d.Attr(d.Create.ResultAttr); found {
				values[a.Name] = types.StringValue(s)
			}
		}
	} else {
		mutated = d.Update
		carryStateIdentity(d, values, state)
		if err := createOnlyChange(d, values, state); err != nil {
			diags.AddError(fmt.Sprintf("Error %s %s", v.gerund, d.Name), err.Error())
			return diags
		}
		if d.Update != nil {
			var extras []url.Values
			if d.Update.DeleteParam {
				if removed := removedParams(d, d.Update, values, state); len(removed) > 0 {
					extras = append(extras, url.Values{"delete": []string{strings.Join(removed, ",")}})
				}
			}
			payload, _, err := runOp(ctx, r.client, d, d.Update, values, extras...)
			if err != nil {
				opError(&diags, v.gerund, d, v.ident(d, values), err)
				return diags
			}
			captureResultAttr(d.Update, payload, values)
		}
	}

	if err := r.applyPending(ctx, mutated, values); err != nil {
		diags.AddError(fmt.Sprintf("Error Applying %s Changes", d.Name), err.Error())
		return diags
	}

	if d.Read != nil {
		if err := r.readBack(ctx, values); err != nil && !(v.tolerateVanished && (err == errVanished || pveclient.IsNotFound(err))) {
			diags.AddError(
				fmt.Sprintf("Error Reading %s After %s", d.Name, v.noun),
				fmt.Sprintf("reading %s (%s): %s", d.Name, v.ident(d, values), err),
			)
			return diags
		}
	}

	id, err := joinID(d, values)
	if err != nil {
		diags.AddError(fmt.Sprintf("Error %s %s", v.gerund, d.Name), err.Error())
		return diags
	}
	if id == "" {
		id = d.Name // singletons: one instance per type
	}
	values["id"] = types.StringValue(id)
	return diags
}

// carryStateIdentity copies identity the plan cannot know from the prior
// state: the id (unknown in the plan until set) and computed path identifiers
// (e.g. an SDN subnet's internal id) the item endpoints need.
func carryStateIdentity(d *TypeDescriptor, values, state map[string]attr.Value) {
	if state == nil {
		return
	}
	if id, ok := state["id"]; ok {
		values["id"] = id
	}
	for _, a := range d.Attributes {
		if a.PathParam == "" || !a.Computed || a.Optional {
			continue
		}
		if cur, ok := values[a.Name]; !ok || cur == nil || cur.IsUnknown() {
			if sv, ok := state[a.Name]; ok {
				values[a.Name] = sv
			}
		}
	}
}
