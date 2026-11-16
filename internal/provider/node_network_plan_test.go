// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestNodeNetworkPlanMethod drives Terraform's plan RPC against the
// registry descriptor: PVE rewrites method/method6 from whether an address
// is sent, so the plan must carry a known derived value.
func TestNodeNetworkPlanMethod(t *testing.T) {
	ctx := context.Background()
	srv := providerserver.NewProtocol6(New("test")())()
	schemaResp, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	failOnErrorDiags(t, schemaResp.Diagnostics)

	reg, err := GeneratedRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var d *TypeDescriptor
	for _, td := range reg.Types {
		if td.Name == "pve_node_network" {
			d = td
		}
	}
	if d == nil {
		t.Fatal("pve_node_network not in registry")
	}

	s := func(v string) attr.Value { return types.StringValue(v) }
	imported := map[string]attr.Value{
		"id": s("pve01/vmbr0"), "node": s("pve01"), "iface": s("vmbr0"), "type": s("bridge"),
		"autostart": types.BoolValue(true), "address": s("10.0.10.10"), "cidr": s("10.0.10.10/24"),
		"netmask": s("24"), "gateway": s("10.0.10.1"), "bridge_ports": s("eth0"),
		"method": s("static"), "method6": s("manual"),
	}
	with := func(base map[string]attr.Value, over map[string]attr.Value) map[string]attr.Value {
		out := map[string]attr.Value{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range over {
			out[k] = v
		}
		return out
	}
	bridgeCfg := map[string]attr.Value{
		"node": s("pve01"), "iface": s("vmbr0"), "type": s("bridge"), "autostart": types.BoolValue(true),
	}

	cases := []struct {
		name          string
		prior, config map[string]attr.Value // prior nil = create
		method        string                // "" = must be unknown
		method6       string
	}{
		{
			name: "imported bridge drops cidr", prior: imported, config: bridgeCfg,
			method: "manual", method6: "manual",
		},
		{
			name: "config unchanged keeps state",
			prior: map[string]attr.Value{
				"id": s("pve01/vmbr1"), "node": s("pve01"), "iface": s("vmbr1"), "type": s("bridge"),
				"autostart": types.BoolValue(true), "method": s("dhcp"), "method6": s("manual"),
			},
			config: map[string]attr.Value{
				"node": s("pve01"), "iface": s("vmbr1"), "type": s("bridge"), "autostart": types.BoolValue(true),
			},
			method: "dhcp", method6: "manual",
		},
		{
			name: "cidr kept stays static", prior: imported,
			config: map[string]attr.Value{
				"node": s("pve01"), "iface": s("vmbr0"), "type": s("bridge"), "autostart": types.BoolValue(false),
				"address": s("10.0.10.10"), "cidr": s("10.0.10.10/24"), "netmask": s("24"),
				"gateway": s("10.0.10.1"), "bridge_ports": s("eth0"),
			},
			method: "static", method6: "manual",
		},
		{
			name: "unknown cidr stays unknown", prior: imported,
			config: with(bridgeCfg, map[string]attr.Value{"cidr": types.StringUnknown()}),
			method: "", method6: "manual",
		},
		{
			name: "create with cidr6", prior: nil,
			config: map[string]attr.Value{
				"node": s("pve01"), "iface": s("vmbr98"), "type": s("bridge"), "cidr6": s("fd00::10/64"),
			},
			method: "manual", method6: "static",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := toTFWith(d, tc.config)
			typ := config.Type()
			prior := tftypes.NewValue(typ, nil)
			if tc.prior != nil {
				prior = toTFWith(d, tc.prior)
			}
			proposed := proposedState(t, typ, prior, config)

			resp, err := srv.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
				TypeName:         "pve_node_network",
				PriorState:       dyn(typ, prior),
				ProposedNewState: dyn(typ, proposed),
				Config:           dyn(typ, config),
			})
			if err != nil {
				t.Fatal(err)
			}
			failOnErrorDiags(t, resp.Diagnostics)
			planned, err := resp.PlannedState.Unmarshal(typ)
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]tftypes.Value{}
			if err := planned.As(&fields); err != nil {
				t.Fatal(err)
			}
			wantKnown(t, "method", fields["method"], tc.method)
			wantKnown(t, "method6", fields["method6"], tc.method6)
		})
	}
}

func dyn(typ tftypes.Type, v tftypes.Value) *tfprotov6.DynamicValue {
	dv, err := tfprotov6.NewDynamicValue(typ, v)
	if err != nil {
		panic(err)
	}
	return &dv
}

// proposedState mimics Terraform core: configured values, plus the prior
// values of the computed-only attributes.
func proposedState(t *testing.T, typ tftypes.Type, prior, config tftypes.Value) tftypes.Value {
	t.Helper()
	if prior.IsNull() {
		return config
	}
	raw := map[string]tftypes.Value{}
	if err := config.As(&raw); err != nil {
		t.Fatal(err)
	}
	cfg := make(map[string]tftypes.Value, len(raw)) // As may alias config's own map
	for k, v := range raw {
		cfg[k] = v
	}
	pri := map[string]tftypes.Value{}
	if err := prior.As(&pri); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"id", "method", "method6"} {
		cfg[name] = pri[name]
	}
	return tftypes.NewValue(typ, cfg)
}

func wantKnown(t *testing.T, name string, v tftypes.Value, want string) {
	t.Helper()
	if want == "" {
		if v.IsKnown() {
			t.Errorf("%s = %s, want unknown", name, v)
		}
		return
	}
	if !v.IsKnown() {
		t.Errorf("%s is unknown, want %q", name, want)
		return
	}
	var got string
	if err := v.As(&got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func failOnErrorDiags(t *testing.T, diags []*tfprotov6.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s", d.Summary, d.Detail)
		}
	}
}
