// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestCollectRefOrigins_exprAny_instanceTraversals(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      string
		expected []string
	}{
		{
			"splat over module instances",
			`attr = module.app[*].url`,
			[]string{"module.app[?].url 1:8-1:25"},
		},
		{
			"module instance picked by a dynamic key",
			`attr = module.app[var.key].url`,
			[]string{"module.app[?].url 1:8-1:31", "var.key 1:19-1:26"},
		},
		{
			"splat over resource instances",
			`attr = aws_instance.web[*].id`,
			[]string{"aws_instance.web[?].id 1:8-1:30"},
		},
		{
			"data source instance picked by a function of a key",
			`attr = data.aws_ami.x[length(local.ids) - 1].id`,
			[]string{"data.aws_ami.x[?].id 1:8-1:48", "local.ids 1:30-1:39"},
		},
		{
			"splat over a list value stays the value",
			`attr = var.list[*].name`,
			[]string{"var.list 1:8-1:16"},
		},
		{
			"dynamic key into an attribute stays the attribute",
			`attr = aws_instance.web.tags[var.k].x`,
			[]string{"aws_instance.web.tags 1:8-1:29", "var.k 1:30-1:35"},
		},
		{
			"splat without an attribute stays the module call",
			`attr = module.app[*]`,
			[]string{"module.app 1:8-1:18"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			bodySchema := &schema.BodySchema{
				Attributes: map[string]*schema.AttributeSchema{
					"attr": {
						Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType},
						IsOptional: true,
					},
				},
			}
			f, diags := hclsyntax.ParseConfig([]byte(tc.cfg), "test.tf", hcl.InitialPos)
			if len(diags) > 0 {
				t.Fatal(diags)
			}
			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files:  map[string]*hcl.File{"test.tf": f},
			})

			origins, err := d.CollectReferenceOrigins()
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(origins))
			for _, origin := range origins {
				lo, ok := origin.(reference.LocalOrigin)
				if !ok {
					t.Fatalf("unexpected origin %#v", origin)
				}
				rng := lo.Range
				got = append(got, fmt.Sprintf("%s %d:%d-%d:%d", addressWithUnknownKeys(lo.Addr),
					rng.Start.Line, rng.Start.Column, rng.End.Line, rng.End.Column))
			}
			if strings.Join(got, ", ") != strings.Join(tc.expected, ", ") {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// addressWithUnknownKeys renders an address with [?] for an unknown key.
func addressWithUnknownKeys(addr lang.Address) string {
	var sb strings.Builder
	for _, step := range addr {
		if idx, ok := step.(lang.IndexStep); ok && !idx.Key.IsKnown() {
			sb.WriteString("[?]")
			continue
		}
		sb.WriteString(step.String())
	}
	return sb.String()
}

func TestCollectRefOrigins_exprAny_ofScopeId(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      string
		expected []string
	}{
		{"a reference of the scope", `provider = local.secondary`, []string{"local.secondary provider"}},
		{"an instance of the scope, picked by a key", `provider = random.by_key[var.k]`, []string{"random.by_key provider", "var.k "}},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			bodySchema := &schema.BodySchema{
				Attributes: map[string]*schema.AttributeSchema{
					"provider": {
						Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType, OfScopeId: lang.ScopeId("provider")},
						IsOptional: true,
					},
				},
			}
			f, diags := hclsyntax.ParseConfig([]byte(tc.cfg), "test.tf", hcl.InitialPos)
			if len(diags) > 0 {
				t.Fatal(diags)
			}
			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files:  map[string]*hcl.File{"test.tf": f},
			})
			origins, err := d.CollectReferenceOrigins()
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(origins))
			for _, origin := range origins {
				lo := origin.(reference.LocalOrigin)
				scopes := make([]string, 0)
				for _, c := range lo.Constraints {
					scopes = append(scopes, string(c.OfScopeId))
				}
				got = append(got, lo.Addr.String()+" "+strings.Join(scopes, ","))
			}
			if strings.Join(got, "; ") != strings.Join(tc.expected, "; ") {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
