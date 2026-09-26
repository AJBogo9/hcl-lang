// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestCompletionAtPos_exprObject_nestedHooks(t *testing.T) {
	objectWithHook := schema.Object{
		Attributes: schema.ObjectAttributes{
			"source": &schema.AttributeSchema{
				Constraint: schema.LiteralType{Type: cty.String},
				CompletionHooks: lang.CompletionHooks{
					{Name: "TestSourceHook"},
				},
			},
			"version": &schema.AttributeSchema{
				Constraint: schema.LiteralType{Type: cty.String},
			},
		},
	}
	hooks := CompletionFuncMap{
		"TestSourceHook": func(ctx context.Context, value cty.Value) ([]Candidate, error) {
			pos, _ := PosFromContext(ctx)
			return []Candidate{
				{
					Label:         fmt.Sprintf("%q", value.AsString()+"/aws"),
					Detail:        fmt.Sprintf("at %d:%d", pos.Line, pos.Column),
					Kind:          lang.StringCandidateKind,
					RawInsertText: fmt.Sprintf("%q", value.AsString()+"/aws"),
				},
			}, nil
		},
	}

	testCases := []struct {
		name       string
		constraint schema.Constraint
		cfg        string
		pos        hcl.Pos
		expected   lang.Candidates
	}{
		{
			"inside the quotes of a nested attribute",
			objectWithHook,
			`attr = { source = "hashicorp" }
`,
			hcl.Pos{Line: 1, Column: 29, Byte: 28},
			lang.IncompleteCandidates([]lang.Candidate{
				{
					Label:  `"hashicorp/aws"`,
					Detail: "at 1:29",
					Kind:   lang.StringCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: `"hashicorp/aws"`,
						Snippet: `"hashicorp/aws"`,
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 19, Byte: 18},
							End:      hcl.Pos{Line: 1, Column: 30, Byte: 29},
						},
					},
				},
			}),
		},
		{
			"empty value after equals sign",
			objectWithHook,
			`attr = {
  source =
}
`,
			hcl.Pos{Line: 2, Column: 11, Byte: 19},
			lang.IncompleteCandidates([]lang.Candidate{
				{
					Label:  `"/aws"`,
					Detail: "at 2:11",
					Kind:   lang.StringCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: `"/aws"`,
						Snippet: `"/aws"`,
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 11, Byte: 19},
							End:      hcl.Pos{Line: 2, Column: 11, Byte: 19},
						},
					},
				},
			}),
		},
		{
			"object inside one of, as in required_providers",
			schema.OneOf{
				objectWithHook,
				schema.LiteralType{Type: cty.String},
			},
			`attr = { source = "x" }
`,
			hcl.Pos{Line: 1, Column: 21, Byte: 20},
			lang.IncompleteCandidates([]lang.Candidate{
				{
					Label:  `"x/aws"`,
					Detail: "at 1:21",
					Kind:   lang.StringCandidateKind,
					TextEdit: lang.TextEdit{
						NewText: `"x/aws"`,
						Snippet: `"x/aws"`,
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 19, Byte: 18},
							End:      hcl.Pos{Line: 1, Column: 22, Byte: 21},
						},
					},
				},
			}),
		},
		{
			"nested attribute without hooks stays complete",
			objectWithHook,
			`attr = { version = "1" }
`,
			hcl.Pos{Line: 1, Column: 22, Byte: 21},
			lang.CompleteCandidates([]lang.Candidate{}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			bodySchema := &schema.BodySchema{
				Attributes: map[string]*schema.AttributeSchema{
					"attr": {Constraint: tc.constraint},
				},
			}
			// some configurations are incomplete on purpose
			f, _ := hclsyntax.ParseConfig([]byte(tc.cfg), "test.tf", hcl.InitialPos)
			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
			})
			for n, h := range hooks {
				d.decoderCtx.CompletionHooks[n] = h
			}

			candidates, err := d.CompletionAtPos(context.Background(), "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.expected, candidates); diff != "" {
				t.Fatalf("unexpected candidates: %s", diff)
			}
		})
	}
}
