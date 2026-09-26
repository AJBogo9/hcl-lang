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
)

// hasSameBodyRefs checks if the given block type has SameBodyRefs enabled
// in the schema using BodyExtensions.
func hasSameBodyRefs(bodySchema *schema.BodySchema, blockType string) bool {
	if bodySchema == nil {
		return false
	}
	blockSchema, ok := bodySchema.Blocks[blockType]
	if !ok || blockSchema.Body == nil || blockSchema.Body.Extensions == nil {
		return false
	}
	return blockSchema.Body.Extensions.SameBodyRefs
}

func (ref Reference) CompletionAtPos(ctx context.Context, pos hcl.Pos) []lang.Candidate {
	if ref.cons.Address != nil {
		// no candidates if traversal itself is addressable
		return []lang.Candidate{}
	}

	if ref.pathCtx.ReferenceTargets == nil {
		return []lang.Candidate{}
	}

	file := ref.pathCtx.Files[ref.expr.Range().Filename]
	rootBody, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return []lang.Candidate{}
	}

	outerBodyRng := rootBody.Range()
	// Find outer block body range to allow filtering
	// of references pointing back to the same block
	outerBlock := rootBody.OutermostBlockAtPos(pos)
	if outerBlock != nil {
		if hasSameBodyRefs(ref.pathCtx.Schema, outerBlock.Type) {
			// Hacky way to allow references to match back to the
			// same block by ignoring outer body range check
			outerBodyRng = hcl.Range{}
		} else {
			ob := outerBlock.Body.(*hclsyntax.Body)
			outerBodyRng = ob.Range()
		}
	} else if attr := rootAttributeAtPos(rootBody, pos); attr != nil {
		// A top-level attribute (such as inputs in a Terragrunt file)
		// may refer to anything in the file but itself.
		outerBodyRng = attr.SrcRange
	}

	if isEmptyExpression(ref.expr) {
		editRng := hcl.Range{
			Filename: ref.expr.Range().Filename,
			Start:    pos,
			End:      pos,
		}
		candidates := make([]lang.Candidate, 0)
		ref.pathCtx.ReferenceTargets.MatchWalk(ctx, ref.cons, "", outerBodyRng, editRng, func(target reference.Target) error {
			address := target.Address(ctx, editRng.Start).String()

			candidates = append(candidates, lang.Candidate{
				Label:       address,
				Detail:      target.FriendlyName(),
				Description: target.Description,
				Kind:        lang.ReferenceCandidateKind,
				TextEdit: lang.TextEdit{
					NewText: address,
					Snippet: address,
					Range:   editRng,
				},
			})
			return nil
		})
		return candidates
	}

	var editRng, prefixRng hcl.Range
	switch eType := ref.expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		editRng = eType.Range()
		if !editRng.ContainsPos(pos) {
			// account for trailing character(s) which doesn't appear in AST
			// such as dot, opening bracket etc.
			editRng.End = pos
		}
		prefixRng = hcl.Range{
			Filename: eType.Range().Filename,
			Start:    eType.Range().Start,
			End:      pos,
		}
	case *hclsyntax.ExprSyntaxError:
		editRng = eType.Range()
		if !editRng.ContainsPos(pos) {
			// account for trailing character(s) which doesn't appear in AST
			// such as dot, opening bracket etc.
			editRng.End = pos
		}
		prefixRng = hcl.Range{
			Filename: eType.Range().Filename,
			Start:    eType.Range().Start,
			End:      pos,
		}
	default:
		return []lang.Candidate{}
	}

	prefix := string(prefixRng.SliceBytes(file.Bytes))

	candidates := make([]lang.Candidate, 0)
	ref.pathCtx.ReferenceTargets.MatchWalk(ctx, ref.cons, prefix, outerBodyRng, editRng, func(target reference.Target) error {
		address := target.Address(ctx, editRng.Start).String()

		candidates = append(candidates, lang.Candidate{
			Label:       address,
			Detail:      target.FriendlyName(),
			Description: target.Description,
			Kind:        lang.ReferenceCandidateKind,
			TextEdit: lang.TextEdit{
				NewText: address,
				Snippet: address,
				Range:   editRng,
			},
		})
		return nil
	})
	return candidates
}

// rootAttributeAtPos returns the attribute of the root body which holds
// pos, if any.
func rootAttributeAtPos(body *hclsyntax.Body, pos hcl.Pos) *hclsyntax.Attribute {
	for _, attr := range body.Attributes {
		if attr.SrcRange.ContainsPos(pos) || attr.SrcRange.End == pos {
			return attr
		}
	}
	return nil
}
