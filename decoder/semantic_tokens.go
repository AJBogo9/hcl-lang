// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"sort"

	"github.com/hashicorp/hcl-lang/decoder/internal/schemahelper"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// SemanticTokensInFile returns a sequence of semantic tokens
// within the config file.
func (d *PathDecoder) SemanticTokensInFile(ctx context.Context, filename string) ([]lang.SemanticToken, error) {
	f, err := d.fileByName(filename)
	if err != nil {
		return nil, err
	}

	body, err := d.bodyForFileAndPos(filename, f, hcl.InitialPos)
	if err != nil {
		return nil, err
	}

	if d.pathCtx.Schema == nil {
		return []lang.SemanticToken{}, nil
	}

	tokens := d.tokensForBody(ctx, body, d.pathCtx.Schema, []lang.SemanticTokenModifier{})

	// TODO decouple semantic tokens for valid references from AST walking
	//   instead of matching targets and origins when encountering a traversal expression,
	//   we can do this way earlier by comparing pathCtx.ReferenceTargets and
	//   d.pathCtx.ReferenceOrigins, to build a list of tokens.
	//   Be sure to sort them afterward!

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Range.Start.Byte < tokens[j].Range.Start.Byte
	})

	return tokens, nil
}

type dynamicBodyCtxKey struct{}

func (d *PathDecoder) tokensForBody(ctx context.Context, body *hclsyntax.Body, bodySchema *schema.BodySchema, parentModifiers []lang.SemanticTokenModifier) []lang.SemanticToken {
	tokens := make([]lang.SemanticToken, 0)

	if bodySchema == nil {
		return d.syntaxTokensForBody(ctx, body)
	}

	sh := d.pathCtx.SemanticHighlighting
	// the body of a dynamic block has meta-arguments (for_each, iterator, labels)
	dynamicBody, _ := ctx.Value(dynamicBodyCtxKey{}).(*hclsyntax.Body)
	isDynamicBody := dynamicBody != nil && dynamicBody == body

	for name, attr := range body.Attributes {
		isMetaArgument := isDynamicBody
		attrSchema, ok := bodySchema.Attributes[name]
		if !ok {
			if bodySchema.Extensions != nil && name == "count" && bodySchema.Extensions.Count {
				attrSchema = schemahelper.CountAttributeSchema()
				isMetaArgument = true
			} else if bodySchema.Extensions != nil && name == "for_each" && bodySchema.Extensions.ForEach {
				attrSchema = schemahelper.ForEachAttributeSchema()
				isMetaArgument = true
			} else {
				if bodySchema.AnyAttribute == nil {
					// unknown attribute
					tokens = append(tokens, syntaxSemanticTokens(ctx, d.pathCtx, attr.Expr)...)
					continue
				}
				attrSchema = bodySchema.AnyAttribute
			}
		}

		attrModifiers := make([]lang.SemanticTokenModifier, 0)
		attrModifiers = append(attrModifiers, parentModifiers...)
		attrModifiers = append(attrModifiers, attrSchema.SemanticTokenModifiers...)
		if sh != nil && isMetaArgument {
			attrModifiers = append(attrModifiers, lang.TokenModifierMetaArgument)
		}

		tokens = append(tokens, lang.SemanticToken{
			Type:      lang.TokenAttrName,
			Modifiers: attrModifiers,
			Range:     attr.NameRange,
		})

		exprTokens := d.newExpression(attr.Expr, attrSchema.Constraint).SemanticTokens(ctx)
		if sh != nil && constraintAllowsReferences(attrSchema.Constraint) {
			exprTokens = mergeSyntaxTokens(exprTokens, syntaxSemanticTokens(ctx, d.pathCtx, attr.Expr))
		}
		tokens = append(tokens, exprTokens...)
	}

	for _, block := range body.Blocks {
		blockSchema, hasDepSchema := bodySchema.Blocks[block.Type]
		if !hasDepSchema {
			// unknown block
			tokens = append(tokens, d.syntaxTokensForBody(ctx, block.Body)...)
			continue
		}

		blockModifiers := make([]lang.SemanticTokenModifier, 0)
		blockModifiers = append(blockModifiers, parentModifiers...)
		blockModifiers = append(blockModifiers, blockSchema.SemanticTokenModifiers...)

		blockCtx := ctx
		blockTypeModifiers := blockModifiers
		isDynamicBlock := sh != nil && block.Type == "dynamic" &&
			bodySchema.Extensions != nil && bodySchema.Extensions.DynamicBlocks
		if isDynamicBlock {
			// only the dynamic keyword itself is a meta-argument,
			// the content block within is ordinary configuration
			blockTypeModifiers = append(blockModifiers[:len(blockModifiers):len(blockModifiers)], lang.TokenModifierMetaArgument)

			var iterTokens []lang.SemanticToken
			blockCtx, iterTokens = d.dynamicBlockIterator(ctx, block)
			tokens = append(tokens, iterTokens...)
			blockCtx = context.WithValue(blockCtx, dynamicBodyCtxKey{}, block.Body)
		}

		tokens = append(tokens, lang.SemanticToken{
			Type:      lang.TokenBlockType,
			Modifiers: blockTypeModifiers,
			Range:     block.TypeRange,
		})

		for i, labelRange := range block.LabelRanges {
			if i+1 > len(blockSchema.Labels) {
				// unknown label
				continue
			}

			labelSchema := blockSchema.Labels[i]

			labelModifiers := make([]lang.SemanticTokenModifier, 0)
			labelModifiers = append(labelModifiers, parentModifiers...)
			labelModifiers = append(labelModifiers, blockSchema.SemanticTokenModifiers...)
			labelModifiers = append(labelModifiers, labelSchema.SemanticTokenModifiers...)

			tokens = append(tokens, lang.SemanticToken{
				Type:      lang.TokenBlockLabel,
				Modifiers: labelModifiers,
				Range:     labelRange,
			})
		}

		if block.Body != nil {
			mergedSchema, _ := schemahelper.MergeBlockBodySchemas(block.AsHCLBlock(), blockSchema)

			tokens = append(tokens, d.tokensForBody(blockCtx, block.Body, mergedSchema, blockModifiers)...)
		}
	}

	return tokens
}

func isPrimitiveTypeDeclaration(kw string) bool {
	switch kw {
	case "bool":
		return true
	case "number":
		return true
	case "string":
		return true
	case "null":
		return true
	case "any":
		return true
	}
	return false
}
