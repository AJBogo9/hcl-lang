// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func (ref Reference) SemanticTokens(ctx context.Context) []lang.SemanticToken {
	eType, ok := ref.expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return []lang.SemanticToken{}
	}

	sh := ref.pathCtx.SemanticHighlighting

	pos := ref.expr.Range().Start
	origins, _ := ref.pathCtx.ReferenceOrigins.AtPos(eType.Range().Filename, pos)

	for _, origin := range origins {
		matchableOrigin, ok := origin.(reference.MatchableOrigin)
		if !ok {
			continue
		}
		targets, ok := ref.pathCtx.ReferenceTargets.Match(matchableOrigin)
		if !ok {
			// target not found
			continue
		}

		if sh == nil {
			return semanticTokensForTraversal(eType.Traversal, nil)
		}
		if kind, ok := ref.scopedReferenceKind(targets); ok {
			return semanticTokensForTraversal(eType.Traversal, &kind)
		}
		if kind, ok := ref.pathCtx.syntaxReferenceKind(ctx, eType.Traversal); ok {
			return semanticTokensForTraversal(eType.Traversal, &kind)
		}
		return semanticTokensForTraversal(eType.Traversal, nil)
	}

	if sh == nil {
		return []lang.SemanticToken{}
	}

	// The reference does not resolve, but its kind may still be known
	// from the constraint (e.g. a provider reference) or from its syntax
	// (e.g. var.name), which is enough to highlight it truthfully.
	if kind, ok := ref.scopedReferenceKind(nil); ok {
		return semanticTokensForTraversal(eType.Traversal, &kind)
	}
	if kind, ok := ref.pathCtx.syntaxReferenceKind(ctx, eType.Traversal); ok {
		return semanticTokensForTraversal(eType.Traversal, &kind)
	}

	return []lang.SemanticToken{}
}

// scopedReferenceKind returns the kind implied by the scope the reference
// is constrained to, or else by the scope shared by all of its targets.
// Only scopes listed in SemanticHighlighting.Scopes are considered, since
// for those the first step does not tell the kind (e.g. local.alias
// referring to a provider named "local").
func (ref Reference) scopedReferenceKind(targets reference.Targets) (schema.ReferenceKind, bool) {
	sh := ref.pathCtx.SemanticHighlighting
	if sh == nil {
		return schema.ReferenceKind{}, false
	}

	if ref.cons.OfScopeId != "" {
		if kind, ok := sh.Scopes[ref.cons.OfScopeId]; ok {
			return kind, true
		}
	}

	if len(targets) == 0 {
		return schema.ReferenceKind{}, false
	}
	scopeId := targets[0].ScopeId
	for _, target := range targets[1:] {
		if target.ScopeId != scopeId {
			return schema.ReferenceKind{}, false
		}
	}
	kind, ok := sh.Scopes[scopeId]
	return kind, ok
}

// semanticTokensForTraversal returns tokens for each step of the traversal.
// When kind is not nil, every attribute-like step carries the kind's
// modifiers plus its role (see schema.ReferenceKind.StepRole).
func semanticTokensForTraversal(traversal hcl.Traversal, kind *schema.ReferenceKind) []lang.SemanticToken {
	return semanticTokensForTraversalFrom(traversal, kind, 0)
}

// semanticTokensForTraversalFrom is semanticTokensForTraversal for
// a traversal whose first attribute-like step is the firstStep-th step
// of the reference (e.g. the steps of a relative traversal).
func semanticTokensForTraversalFrom(traversal hcl.Traversal, kind *schema.ReferenceKind, firstStep int) []lang.SemanticToken {
	tokens := make([]lang.SemanticToken, 0)

	stepModifiers := func(i int) []lang.SemanticTokenModifier {
		if kind == nil {
			return []lang.SemanticTokenModifier{}
		}
		modifiers := make([]lang.SemanticTokenModifier, 0, len(kind.Modifiers)+1)
		modifiers = append(modifiers, kind.Modifiers...)
		return append(modifiers, kind.StepRole(i))
	}

	step := firstStep
	for _, t := range traversal {
		switch ts := t.(type) {
		case hcl.TraverseRoot:
			tokens = append(tokens, lang.SemanticToken{
				Type:      lang.TokenReferenceStep,
				Modifiers: stepModifiers(step),
				Range:     t.SourceRange(),
			})
			step++
		case hcl.TraverseAttr:
			rng := t.SourceRange()
			tokens = append(tokens, lang.SemanticToken{
				Type:      lang.TokenReferenceStep,
				Modifiers: stepModifiers(step),
				Range: hcl.Range{
					Filename: rng.Filename,
					// omit the initial '.'
					Start: hcl.Pos{
						Line:   rng.Start.Line,
						Column: rng.Start.Column + 1,
						Byte:   rng.Start.Byte + 1,
					},
					End: rng.End,
				},
			})
			step++
		case hcl.TraverseIndex:
			// for index steps we only report
			// what's inside brackets
			rng := t.SourceRange()
			idxRange := hcl.Range{
				Filename: rng.Filename,
				Start: hcl.Pos{
					Line:   rng.Start.Line,
					Column: rng.Start.Column + 1,
					Byte:   rng.Start.Byte + 1,
				},
				End: hcl.Pos{
					Line:   rng.End.Line,
					Column: rng.End.Column - 1,
					Byte:   rng.End.Byte - 1,
				},
			}

			if ts.Key.Type() == cty.String {
				tokens = append(tokens, lang.SemanticToken{
					Type:      lang.TokenMapKey,
					Modifiers: []lang.SemanticTokenModifier{},
					Range:     idxRange,
				})
			}
			if ts.Key.Type() == cty.Number {
				tokens = append(tokens, lang.SemanticToken{
					Type:      lang.TokenNumber,
					Modifiers: []lang.SemanticTokenModifier{},
					Range:     idxRange,
				})
			}
		}
	}

	return tokens
}

// traversalStepCount returns the number of attribute-like
// (root and attribute) steps in the traversal.
func traversalStepCount(traversal hcl.Traversal) int {
	count := 0
	for _, t := range traversal {
		switch t.(type) {
		case hcl.TraverseRoot, hcl.TraverseAttr:
			count++
		}
	}
	return count
}
