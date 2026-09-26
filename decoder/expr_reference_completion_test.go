// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestCompletionAtPos_exprReference(t *testing.T) {
	testCases := []struct {
		testName           string
		attrSchema         map[string]*schema.AttributeSchema
		refTargets         reference.Targets
		cfg                string
		pos                hcl.Pos
		expectedCandidates lang.Candidates
	}{
		{
			"no expression",
			map[string]*schema.AttributeSchema{
				"attr": {
					Constraint: schema.Reference{
						OfType: cty.String,
					},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "foo"},
					},
					Type: cty.String,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.List(cty.Number),
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "baz"},
					},
					Type: cty.Number,
				},
			},
			`attr = `,
			hcl.Pos{Line: 1, Column: 8, Byte: 7},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "local.foo",
					Detail: "string",
					Kind:   lang.ReferenceCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: "local.foo",
						Snippet: "local.foo",
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
							End:      hcl.Pos{Line: 1, Column: 8, Byte: 7},
						},
					},
				},
				{
					Label:  "local.baz",
					Detail: "number",
					Kind:   lang.ReferenceCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: "local.baz",
						Snippet: "local.baz",
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
							End:      hcl.Pos{Line: 1, Column: 8, Byte: 7},
						},
					},
				},
			}),
		},
		{
			"matching prefix",
			map[string]*schema.AttributeSchema{
				"attr": {
					Constraint: schema.Reference{
						OfType: cty.Number,
					},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "foo"},
					},
					Type: cty.List(cty.String),
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "data"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
			},
			`attr = local`,
			hcl.Pos{Line: 1, Column: 13, Byte: 12},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "local.bar",
					Detail: "number",
					Kind:   lang.ReferenceCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: "local.bar",
						Snippet: "local.bar",
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
							End:      hcl.Pos{Line: 1, Column: 13, Byte: 12},
						},
					},
				},
			}),
		},
		{
			"matching prefix in the middle",
			map[string]*schema.AttributeSchema{
				"attr": {
					Constraint: schema.Reference{
						OfType: cty.String,
					},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "foo"},
					},
					Type: cty.String,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.List(cty.Number),
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "data"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
			},
			`attr = local`,
			hcl.Pos{Line: 1, Column: 11, Byte: 10},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "local.foo",
					Detail: "string",
					Kind:   lang.ReferenceCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: "local.foo",
						Snippet: "local.foo",
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
							End:      hcl.Pos{Line: 1, Column: 13, Byte: 12},
						},
					},
				},
			}),
		},
		{
			"matching prefix after trailing dot",
			map[string]*schema.AttributeSchema{
				"attr": {
					Constraint: schema.Reference{
						OfType: cty.String,
					},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "foo"},
					},
					Type: cty.String,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.List(cty.Number),
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "data"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
			},
			`attr = local.`,
			hcl.Pos{Line: 1, Column: 14, Byte: 13},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "local.foo",
					Detail: "string",
					Kind:   lang.ReferenceCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: "local.foo",
						Snippet: "local.foo",
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
							End:      hcl.Pos{Line: 1, Column: 14, Byte: 13},
						},
					},
				},
			}),
		},
		{
			"mismatching prefix",
			map[string]*schema.AttributeSchema{
				"attr": {
					Constraint: schema.Reference{
						OfType: cty.Number,
					},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "foo"},
					},
					Type: cty.String,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
				{
					Addr: lang.Address{
						lang.RootStep{Name: "data"},
						lang.AttrStep{Name: "bar"},
					},
					Type: cty.Number,
				},
			},
			`attr = x`,
			hcl.Pos{Line: 1, Column: 9, Byte: 8},
			lang.CompleteCandidates([]lang.Candidate{}),
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.testName), func(t *testing.T) {
			bodySchema := &schema.BodySchema{
				Attributes: tc.attrSchema,
			}

			f, _ := hclsyntax.ParseConfig([]byte(tc.cfg), "test.tf", hcl.InitialPos)
			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
				ReferenceTargets: tc.refTargets,
			})

			ctx := context.Background()
			candidates, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tc.expectedCandidates, candidates); diff != "" {
				t.Fatalf("unexpected candidates: %s", diff)
			}
		})
	}
}

// A top-level attribute, such as inputs in a Terragrunt file, completes
// references to what the file declares elsewhere, but not to itself.
func TestCompletionAtPos_exprReference_topLevelAttribute(t *testing.T) {
	cfg := `locals {
  region = "x"
}
inputs = local.
other = 1
`
	rangeOf := func(start, end hcl.Pos) *hcl.Range {
		return &hcl.Range{Filename: "test.tf", Start: start, End: end}
	}
	targets := reference.Targets{
		{
			Addr:     lang.Address{lang.RootStep{Name: "local"}, lang.AttrStep{Name: "region"}},
			Type:     cty.String,
			RangePtr: rangeOf(hcl.Pos{Line: 2, Column: 3, Byte: 11}, hcl.Pos{Line: 2, Column: 15, Byte: 23}),
		},
		{
			// declared by the attribute being completed
			Addr:     lang.Address{lang.RootStep{Name: "local"}, lang.AttrStep{Name: "self"}},
			Type:     cty.String,
			RangePtr: rangeOf(hcl.Pos{Line: 4, Column: 1, Byte: 26}, hcl.Pos{Line: 4, Column: 16, Byte: 41}),
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"locals": {Body: &schema.BodySchema{AnyAttribute: &schema.AttributeSchema{Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}}}},
		},
		Attributes: map[string]*schema.AttributeSchema{
			"inputs": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}, IsOptional: true},
			"other":  {Constraint: schema.AnyExpression{OfType: cty.Number}, IsOptional: true},
		},
	}

	f, _ := hclsyntax.ParseConfig([]byte(cfg), "test.tf", hcl.InitialPos)
	d := testPathDecoder(t, &PathContext{
		Schema:           bodySchema,
		Files:            map[string]*hcl.File{"test.tf": f},
		ReferenceTargets: targets,
	})

	candidates, err := d.CompletionAtPos(context.Background(), "test.tf", hcl.Pos{Line: 4, Column: 16, Byte: 41})
	if err != nil {
		t.Fatal(err)
	}
	labels := make([]string, 0)
	for _, c := range candidates.List {
		labels = append(labels, c.Label)
	}
	if diff := cmp.Diff([]string{"local.region"}, labels); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}
