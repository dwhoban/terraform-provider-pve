// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// genericFunction implements every generated provider function: one GET
// endpoint with the arguments as query parameters.
type genericFunction struct {
	engineBase
}

var _ function.Function = (*genericFunction)(nil)

// NewGenericFunction returns the function constructor for a descriptor.
func NewGenericFunction(d *TypeDescriptor) func() function.Function {
	return func() function.Function { return &genericFunction{engineBase: engineBase{desc: d}} }
}

func (f *genericFunction) Metadata(ctx context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = strings.TrimPrefix(f.desc.Name, "pve_")
}

func (f *genericFunction) Definition(ctx context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
	params := make([]function.Parameter, 0, len(f.desc.FuncArgs))
	for _, a := range f.desc.FuncArgs {
		desc := a.Description
		if len(a.Enum) > 0 {
			desc += " One of: " + joinQuoted(a.Enum) + "."
		}
		switch a.Type {
		case AttrBool:
			params = append(params, function.BoolParameter{Name: a.Name, Description: desc})
		case AttrInt64:
			params = append(params, function.Int64Parameter{Name: a.Name, Description: desc})
		case AttrFloat64:
			params = append(params, function.Float64Parameter{Name: a.Name, Description: desc})
		default:
			params = append(params, function.StringParameter{Name: a.Name, Description: desc})
		}
	}
	var result function.Return
	switch f.desc.FuncResult {
	case AttrInt64:
		result = function.Int64Return{}
	case AttrBool:
		result = function.BoolReturn{}
	case AttrFloat64:
		result = function.Float64Return{}
	default:
		result = function.StringReturn{}
	}
	resp.Definition = function.Definition{
		Summary:             f.desc.Summary,
		MarkdownDescription: f.desc.Summary,
		Parameters:          params,
		Return:              result,
	}
}

func (f *genericFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	// Framework v1.19.0 has no per-function configure hook; pick up the
	// client the provider instance stored during Configure.
	if f.client == nil {
		f.client = sharedProviderClient.get()
	}
	if err := f.requireClient(); err != nil {
		resp.Error = function.NewFuncError(err.Error())
		return
	}
	values := make(map[string]attr.Value, len(f.desc.FuncArgs))
	for i, a := range f.desc.FuncArgs {
		v, ferr := getArgument(ctx, req.Arguments, i, a)
		if ferr != nil {
			resp.Error = ferr
			return
		}
		values[a.Name] = v
	}
	payload, _, err := runOp(ctx, f.client, f.desc, f.desc.Call, values)
	if err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("%s: %s", f.desc.Name, err))
		return
	}
	resultAttr := Attr{Name: "result", Type: f.desc.FuncResult, Attrs: f.desc.FuncResultAttrs}
	var decoded any
	if len(payload) > 0 {
		decoded, err = decodeJSON(payload)
		if err != nil {
			resp.Error = function.NewFuncError(fmt.Sprintf("%s: decoding response: %s", f.desc.Name, err))
			return
		}
	}
	result, err := decodeAttr(resultAttr, decoded)
	if err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("%s: %s", f.desc.Name, err))
		return
	}
	resp.Error = resp.Result.Set(ctx, result)
}

func getArgument(ctx context.Context, args function.ArgumentsData, pos int, a Attr) (attr.Value, *function.FuncError) {
	var target any
	switch a.Type {
	case AttrBool:
		target = &types.Bool{}
	case AttrInt64:
		target = &types.Int64{}
	case AttrFloat64:
		target = &types.Float64{}
	default:
		target = &types.String{}
	}
	if err := args.GetArgument(ctx, pos, target); err != nil {
		return nil, err
	}
	return reflectValueOf(target), nil
}

func reflectValueOf(target any) attr.Value {
	switch t := target.(type) {
	case *types.Bool:
		return *t
	case *types.Int64:
		return *t
	case *types.Float64:
		return *t
	case *types.String:
		return *t
	default:
		return types.StringNull()
	}
}
