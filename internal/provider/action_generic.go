// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// genericAction implements every generated action: one mutating endpoint,
// task-waited when it answers with a UPID, with progress events.
type genericAction struct {
	engineBase
}

var (
	_ action.Action              = (*genericAction)(nil)
	_ action.ActionWithConfigure = (*genericAction)(nil)
)

// NewGenericAction returns the action constructor for a descriptor.
func NewGenericAction(d *TypeDescriptor) func() action.Action {
	return func() action.Action { return &genericAction{engineBase: engineBase{desc: d}} }
}

func (a *genericAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = a.desc.Name
}

func (a *genericAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = buildActionSchema(a.desc)
}

func (a *genericAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.setClient(req.ProviderData)
}

func (a *genericAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	values := configValues(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var payload json.RawMessage
	var waited bool
	if err := a.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Invoking %s (%s)", a.desc.Name, describeValues(a.desc, values))})
	}
	op := a.desc.Invoke
	path, err := resolvePath(a.desc, op, values, nil)
	var params url.Values
	if err == nil {
		params, err = buildParams(a.desc, op, values)
	}
	if err == nil {
		start := time.Now()
		lastReport := time.Time{}
		payload, waited, err = executeOp(ctx, a.client, op, path, params, func(elapsed time.Duration, status string) {
			// Poll fast, report slow: progress events cross the plugin
			// protocol, so throttle them instead of emitting per poll.
			if resp.SendProgress == nil || time.Since(lastReport) < 30*time.Second {
				return
			}
			lastReport = time.Now()
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("%s: task running (elapsed %s, status %q)", a.desc.Name, time.Since(start).Round(time.Second), status),
			})
		})
	}
	if err != nil {
		opError(&resp.Diagnostics, "Invoking", a.desc, describeValues(a.desc, values), err)
		return
	}
	if resp.SendProgress != nil {
		msg := fmt.Sprintf("%s completed", a.desc.Name)
		if waited {
			msg += " (task exited OK)"
		} else if s, ok := payloadString(payload); ok {
			msg += ": " + truncate(s, 200)
		} else if len(payload) > 0 {
			msg += ": " + truncate(string(payload), 200)
		}
		resp.SendProgress(action.InvokeProgressEvent{Message: msg})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
