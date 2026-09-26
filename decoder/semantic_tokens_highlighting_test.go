// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

var testSemanticHighlighting = &schema.SemanticHighlighting{
	RootNames: map[string]schema.ReferenceKind{
		"var":   {Modifiers: lang.SemanticTokenModifiers{"kind-var"}, KeywordSteps: 1, NameSteps: 1},
		"local": {Modifiers: lang.SemanticTokenModifiers{"kind-local"}, KeywordSteps: 1, NameSteps: 1},
		"data":  {Modifiers: lang.SemanticTokenModifiers{"kind-data"}, KeywordSteps: 1, TypeSteps: 1, NameSteps: 1},
		"each":  {Modifiers: lang.SemanticTokenModifiers{"kind-iter"}, KeywordSteps: 1, NameSteps: 1},
		"self":  {Modifiers: lang.SemanticTokenModifiers{"kind-iter"}, KeywordSteps: 1},
	},
	Scopes: map[lang.ScopeId]schema.ReferenceKind{
		"provider": {Modifiers: lang.SemanticTokenModifiers{"kind-provider"}, TypeSteps: 1, NameSteps: 1},
	},
	Default:           &schema.ReferenceKind{Modifiers: lang.SemanticTokenModifiers{"kind-resource"}, TypeSteps: 1, NameSteps: 1},
	IteratorModifiers: lang.SemanticTokenModifiers{"kind-iter"},
}

func TestSemanticTokens_highlighting(t *testing.T) {
	anyAttr := map[string]*schema.AttributeSchema{
		"attr": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
	}

	testCases := []struct {
		testName       string
		bodySchema     *schema.BodySchema
		refOrigins     reference.Origins
		refTargets     reference.Targets
		highlighting   *schema.SemanticHighlighting
		cfg            string
		expectedTokens []string
	}{
		{
			"unresolved variable reference by root name",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = var.foo.bar`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:12 foo hcl-referenceStep[kind-var,hcl-nameStep]",
				"1:16 bar hcl-referenceStep[kind-var,hcl-attrStep]",
			},
		},
		{
			"references interpolated into a literal string attribute",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"msg": {Constraint: schema.LiteralType{Type: cty.String}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`msg = "Got ${var.cfg.name}."`,
			[]string{
				"1:1 msg hcl-attrName[]",
				"1:14 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:18 cfg hcl-referenceStep[kind-var,hcl-nameStep]",
				"1:22 name hcl-referenceStep[kind-var,hcl-attrStep]",
			},
		},
		{
			"content of a meta-argument block does not inherit its modifier",
			&schema.BodySchema{Blocks: map[string]*schema.BlockSchema{
				"lifecycle": {
					SemanticTokenModifiers: lang.SemanticTokenModifiers{lang.TokenModifierMetaArgument},
					Body: &schema.BodySchema{
						Attributes: map[string]*schema.AttributeSchema{
							"keep": {
								Constraint:             schema.LiteralType{Type: cty.Bool},
								SemanticTokenModifiers: lang.SemanticTokenModifiers{lang.TokenModifierMetaArgument},
							},
						},
						Blocks: map[string]*schema.BlockSchema{
							"check": {Body: &schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
								"cond": {Constraint: schema.LiteralType{Type: cty.Bool}},
							}}},
						},
					},
				},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`lifecycle {
  keep = true
  check {
    cond = true
  }
}
`,
			[]string{
				"1:1 lifecycle hcl-blockType[hcl-metaArgument]",
				"2:3 keep hcl-attrName[hcl-metaArgument]",
				"2:10 true hcl-bool[]",
				"3:3 check hcl-blockType[]",
				"4:5 cond hcl-attrName[]",
				"4:12 true hcl-bool[]",
			},
		},
		{
			"unresolved data source reference",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = data.aws_ami.web.id`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 data hcl-referenceStep[kind-data,hcl-keywordStep]",
				"1:13 aws_ami hcl-referenceStep[kind-data,hcl-typeStep]",
				"1:21 web hcl-referenceStep[kind-data,hcl-nameStep]",
				"1:25 id hcl-referenceStep[kind-data,hcl-attrStep]",
			},
		},
		{
			"unresolved reference with index steps uses the default kind",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = aws_instance.web[0].id`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 aws_instance hcl-referenceStep[kind-resource,hcl-typeStep]",
				"1:21 web hcl-referenceStep[kind-resource,hcl-nameStep]",
				"1:25 0 hcl-number[]",
				"1:28 id hcl-referenceStep[kind-resource,hcl-attrStep]",
			},
		},
		{
			"single unknown step is not a reference of a known kind",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = foo`,
			[]string{
				"1:1 attr hcl-attrName[]",
			},
		},
		{
			"object constructor and function call in a dynamic-typed place",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = merge(local.a, { key = var.b, "quoted" = 1, (var.c) = 2 })`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 merge hcl-functionName[]",
				"1:14 local hcl-referenceStep[kind-local,hcl-keywordStep]",
				"1:20 a hcl-referenceStep[kind-local,hcl-nameStep]",
				"1:25 key hcl-objectKey[]",
				"1:31 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:35 b hcl-referenceStep[kind-var,hcl-nameStep]",
				"1:53 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:57 c hcl-referenceStep[kind-var,hcl-nameStep]",
			},
		},
		{
			"for expression keywords and iterator symbols",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = { for k, v in var.m : k => v.name if v.on }`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:10 for hcl-keyword[]",
				"1:14 k hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:17 v hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:19 in hcl-keyword[]",
				"1:22 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:26 m hcl-referenceStep[kind-var,hcl-nameStep]",
				"1:30 k hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:35 v hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:37 name hcl-referenceStep[kind-iter,hcl-attrStep]",
				"1:42 if hcl-keyword[]",
				"1:45 v hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:47 on hcl-referenceStep[kind-iter,hcl-attrStep]",
			},
		},
		{
			"typed for expression scopes its iterator symbols",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"attr": {Constraint: schema.AnyExpression{OfType: cty.List(cty.String)}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = [for s in var.l : s.name]`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:9 for hcl-keyword[]",
				"1:13 s hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:15 in hcl-keyword[]",
				"1:18 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:22 l hcl-referenceStep[kind-var,hcl-nameStep]",
				"1:26 s hcl-referenceStep[kind-iter,hcl-nameStep]",
				"1:28 name hcl-referenceStep[kind-iter,hcl-attrStep]",
			},
		},
		{
			"splat and relative traversal keep the kind of their source",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"attr":  {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
				"other": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = aws_instance.web[*].id
other = local.subnets[each.key].cidr
`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 aws_instance hcl-referenceStep[kind-resource,hcl-typeStep]",
				"1:21 web hcl-referenceStep[kind-resource,hcl-nameStep]",
				"1:28 id hcl-referenceStep[kind-resource,hcl-attrStep]",
				"2:1 other hcl-attrName[]",
				"2:9 local hcl-referenceStep[kind-local,hcl-keywordStep]",
				"2:15 subnets hcl-referenceStep[kind-local,hcl-nameStep]",
				"2:23 each hcl-referenceStep[kind-iter,hcl-keywordStep]",
				"2:28 key hcl-referenceStep[kind-iter,hcl-nameStep]",
				"2:33 cidr hcl-referenceStep[kind-local,hcl-attrStep]",
			},
		},
		{
			"unknown attributes and blocks are tokenized from syntax",
			&schema.BodySchema{},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`unknown = var.a
blk {
  nested = upper(local.b)
}
`,
			[]string{
				"1:11 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"1:15 a hcl-referenceStep[kind-var,hcl-nameStep]",
				"3:12 upper hcl-functionName[]",
				"3:18 local hcl-referenceStep[kind-local,hcl-keywordStep]",
				"3:24 b hcl-referenceStep[kind-local,hcl-nameStep]",
			},
		},
		{
			"type declarations and literal values are not tokenized from syntax",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"type":  {Constraint: schema.TypeDeclaration{}},
				"attr":  {Constraint: schema.LiteralType{Type: cty.String}},
				"paths": {Constraint: schema.Set{}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`type = list(string)
attr = var.not_allowed
paths = [tags.name]
`,
			[]string{
				"1:1 type hcl-attrName[]",
				"1:8 list hcl-typeComplex[]",
				"1:13 string hcl-typePrimitive[]",
				"2:1 attr hcl-attrName[]",
				"3:1 paths hcl-attrName[]",
			},
		},
		{
			"optional object attributes in type declarations",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"type": {Constraint: schema.TypeDeclaration{}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`type = object({ a = optional(number, 8), b = optional(list(string)) })`,
			[]string{
				"1:1 type hcl-attrName[]",
				"1:8 object hcl-typeComplex[]",
				"1:17 a hcl-attrName[]",
				"1:21 optional hcl-typeComplex[]",
				"1:30 number hcl-typePrimitive[]",
				"1:38 8 hcl-number[]",
				"1:42 b hcl-attrName[]",
				"1:46 optional hcl-typeComplex[]",
				"1:55 list hcl-typeComplex[]",
				"1:60 string hcl-typePrimitive[]",
			},
		},
		{
			"meta-arguments of body extensions",
			&schema.BodySchema{Blocks: map[string]*schema.BlockSchema{
				"res": {Body: &schema.BodySchema{
					Extensions: &schema.BodyExtensions{Count: true, ForEach: true},
				}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`res {
  count    = 1
  for_each = var.m
}
`,
			[]string{
				"1:1 res hcl-blockType[]",
				"2:3 count hcl-attrName[hcl-metaArgument]",
				"2:14 1 hcl-number[]",
				"3:3 for_each hcl-attrName[hcl-metaArgument]",
				"3:14 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"3:18 m hcl-referenceStep[kind-var,hcl-nameStep]",
			},
		},
		{
			"dynamic block iterator and meta-arguments",
			&schema.BodySchema{Blocks: map[string]*schema.BlockSchema{
				"res": {Body: &schema.BodySchema{
					Extensions: &schema.BodyExtensions{DynamicBlocks: true},
					Blocks: map[string]*schema.BlockSchema{
						"rule": {Body: &schema.BodySchema{
							Attributes: map[string]*schema.AttributeSchema{
								"port": {Constraint: schema.AnyExpression{OfType: cty.Number}},
							},
						}},
					},
				}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`res {
  dynamic "rule" {
    for_each = var.rules
    iterator = r
    content {
      port = r.value.port
    }
  }
}
`,
			[]string{
				"1:1 res hcl-blockType[]",
				"2:3 dynamic hcl-blockType[hcl-metaArgument]",
				"2:11 \"rule\" hcl-blockLabel[]",
				"3:5 for_each hcl-attrName[hcl-metaArgument]",
				"3:16 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"3:20 rules hcl-referenceStep[kind-var,hcl-nameStep]",
				"4:5 iterator hcl-attrName[hcl-metaArgument]",
				"4:16 r hcl-referenceStep[kind-iter,hcl-keywordStep]",
				"5:5 content hcl-blockType[]",
				"6:7 port hcl-attrName[]",
				"6:14 r hcl-referenceStep[kind-iter,hcl-keywordStep]",
				"6:16 value hcl-referenceStep[kind-iter,hcl-nameStep]",
				"6:22 port hcl-referenceStep[kind-iter,hcl-attrStep]",
			},
		},
		{
			"dynamic block iterator in a body without schema",
			&schema.BodySchema{},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`res {
  dynamic "rule" {
    for_each = var.rules
    iterator = r
    content {
      port = r.value.port
    }
  }
}
`,
			[]string{
				"3:16 var hcl-referenceStep[kind-var,hcl-keywordStep]",
				"3:20 rules hcl-referenceStep[kind-var,hcl-nameStep]",
				"4:16 r hcl-referenceStep[kind-iter,hcl-keywordStep]",
				"6:14 r hcl-referenceStep[kind-iter,hcl-keywordStep]",
				"6:16 value hcl-referenceStep[kind-iter,hcl-nameStep]",
				"6:22 port hcl-referenceStep[kind-iter,hcl-attrStep]",
			},
		},
		{
			"resolved reference whose targets are all in a scope with a kind",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{
				reference.LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "west"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 18, Byte: 17},
					},
					Constraints: reference.OriginConstraints{{OfType: cty.DynamicPseudoType}},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "local"},
						lang.AttrStep{Name: "west"},
					},
					ScopeId: "provider",
					Type:    cty.DynamicPseudoType,
					RangePtr: &hcl.Range{
						Filename: "other.tf",
						Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
						End:      hcl.Pos{Line: 1, Column: 2, Byte: 1},
					},
				},
			},
			testSemanticHighlighting,
			`attr = local.west`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 local hcl-referenceStep[kind-provider,hcl-typeStep]",
				"1:14 west hcl-referenceStep[kind-provider,hcl-nameStep]",
			},
		},
		{
			"unresolved reference constrained to a scope with a kind",
			&schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"attr": {Constraint: schema.Reference{OfScopeId: "provider"}},
			}},
			reference.Origins{},
			reference.Targets{},
			testSemanticHighlighting,
			`attr = local.east`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 local hcl-referenceStep[kind-provider,hcl-typeStep]",
				"1:14 east hcl-referenceStep[kind-provider,hcl-nameStep]",
			},
		},
		{
			"resolved reference without highlighting keeps plain steps",
			&schema.BodySchema{Attributes: anyAttr},
			reference.Origins{
				reference.LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "foo"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 15, Byte: 14},
					},
					Constraints: reference.OriginConstraints{{OfType: cty.DynamicPseudoType}},
				},
			},
			reference.Targets{
				{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "foo"},
					},
					ScopeId: "variable",
					Type:    cty.String,
					RangePtr: &hcl.Range{
						Filename: "other.tf",
						Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
						End:      hcl.Pos{Line: 1, Column: 2, Byte: 1},
					},
				},
			},
			nil,
			`attr = var.foo
other = { a = var.foo }
`,
			[]string{
				"1:1 attr hcl-attrName[]",
				"1:8 var hcl-referenceStep[]",
				"1:12 foo hcl-referenceStep[]",
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.testName), func(t *testing.T) {
			f, _ := hclsyntax.ParseConfig([]byte(tc.cfg), "test.tf", hcl.InitialPos)
			d := testPathDecoder(t, &PathContext{
				Schema: tc.bodySchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
				ReferenceOrigins:     tc.refOrigins,
				ReferenceTargets:     tc.refTargets,
				SemanticHighlighting: tc.highlighting,
			})

			ctx := context.Background()
			tokens, err := d.SemanticTokensInFile(ctx, "test.tf")
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tc.expectedTokens, formatSemanticTokens([]byte(tc.cfg), tokens)); diff != "" {
				t.Fatalf("unexpected tokens: %s", diff)
			}
		})
	}
}

func TestMergeSyntaxTokens(t *testing.T) {
	tok := func(start, end int) lang.SemanticToken {
		return lang.SemanticToken{
			Type: lang.TokenReferenceStep,
			Range: hcl.Range{
				Start: hcl.Pos{Line: 1, Column: start + 1, Byte: start},
				End:   hcl.Pos{Line: 1, Column: end + 1, Byte: end},
			},
		}
	}
	testCases := []struct {
		name     string
		typed    []lang.SemanticToken
		syntax   []lang.SemanticToken
		expected []lang.SemanticToken
	}{
		{"no syntax tokens", []lang.SemanticToken{tok(0, 3)}, nil, []lang.SemanticToken{tok(0, 3)}},
		{"no typed tokens", nil, []lang.SemanticToken{tok(0, 3)}, []lang.SemanticToken{tok(0, 3)}},
		{
			"overlapping syntax tokens are dropped, adjacent ones kept",
			[]lang.SemanticToken{tok(10, 20), tok(0, 5)},
			[]lang.SemanticToken{tok(0, 5), tok(5, 10), tok(15, 16), tok(19, 25), tok(20, 21)},
			[]lang.SemanticToken{tok(10, 20), tok(0, 5), tok(5, 10), tok(20, 21)},
		},
		{
			"nested typed tokens",
			[]lang.SemanticToken{tok(0, 30), tok(5, 8)},
			[]lang.SemanticToken{tok(25, 28), tok(30, 31)},
			[]lang.SemanticToken{tok(0, 30), tok(5, 8), tok(30, 31)},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			typed := append([]lang.SemanticToken{}, tc.typed...)
			if diff := cmp.Diff(tc.expected, mergeSyntaxTokens(typed, tc.syntax)); diff != "" {
				t.Fatalf("unexpected tokens: %s", diff)
			}
		})
	}
}

func TestReferenceKind_StepRole(t *testing.T) {
	kind := schema.ReferenceKind{KeywordSteps: 1, TypeSteps: 1, NameSteps: 1}
	expected := []lang.SemanticTokenModifier{
		lang.TokenModifierKeywordStep,
		lang.TokenModifierTypeStep,
		lang.TokenModifierNameStep,
		lang.TokenModifierAttrStep,
		lang.TokenModifierAttrStep,
	}
	for i, want := range expected {
		if got := kind.StepRole(i); got != want {
			t.Errorf("step %d: expected %q, got %q", i, want, got)
		}
	}
}

// formatSemanticTokens renders tokens as "line:column text type[modifiers]"
func formatSemanticTokens(src []byte, tokens []lang.SemanticToken) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		modifiers := make([]string, 0, len(t.Modifiers))
		for _, m := range t.Modifiers {
			modifiers = append(modifiers, string(m))
		}
		out = append(out, fmt.Sprintf("%d:%d %s %s[%s]", t.Range.Start.Line, t.Range.Start.Column,
			src[t.Range.Start.Byte:t.Range.End.Byte], t.Type, strings.Join(modifiers, ",")))
	}
	return out
}

func TestSemanticTokensInLines(t *testing.T) {
	cfg := `attr = var.one
res {
  attr = var.two
}
res {
  attr = var.three
}
attr2 = var.four
`
	bodySchema := &schema.BodySchema{
		Attributes: map[string]*schema.AttributeSchema{
			"attr":  {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
			"attr2": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
		},
		Blocks: map[string]*schema.BlockSchema{
			"res": {Body: &schema.BodySchema{Attributes: map[string]*schema.AttributeSchema{
				"attr": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
			}}},
		},
	}
	f, _ := hclsyntax.ParseConfig([]byte(cfg), "test.tf", hcl.InitialPos)
	d := testPathDecoder(t, &PathContext{
		Schema:               bodySchema,
		Files:                map[string]*hcl.File{"test.tf": f},
		ReferenceOrigins:     reference.Origins{},
		ReferenceTargets:     reference.Targets{},
		SemanticHighlighting: testSemanticHighlighting,
	})
	ctx := context.Background()
	full, err := d.SemanticTokensInFile(ctx, "test.tf")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		startLine, endLine int
	}{
		{1, 1},
		{2, 3},
		{3, 5},
		{6, 8},
		{1, 8},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%d", tc.startLine, tc.endLine), func(t *testing.T) {
			got, err := d.SemanticTokensInLines(ctx, "test.tf", tc.startLine, tc.endLine)
			if err != nil {
				t.Fatal(err)
			}
			// the same tokens as the whole file on those lines, and no
			// tokens of attributes or blocks far from them
			onLines := func(tokens []lang.SemanticToken) []string {
				kept := make([]lang.SemanticToken, 0)
				for _, tok := range tokens {
					if tok.Range.End.Line >= tc.startLine && tok.Range.Start.Line <= tc.endLine {
						kept = append(kept, tok)
					}
				}
				return formatSemanticTokens([]byte(cfg), kept)
			}
			if diff := cmp.Diff(onLines(full), onLines(got)); diff != "" {
				t.Fatalf("unexpected tokens on the lines: %s", diff)
			}
			for _, tok := range got {
				if tok.Range.Start.Line < tc.startLine-3 || tok.Range.Start.Line > tc.endLine+3 {
					t.Fatalf("decoded a far token at line %d", tok.Range.Start.Line)
				}
			}
		})
	}
}
