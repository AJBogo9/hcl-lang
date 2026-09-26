// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"sort"

	"github.com/hashicorp/hcl-lang/decoder/internal/ast"
	"github.com/hashicorp/hcl-lang/decoder/internal/schemahelper"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

func (d *Decoder) ReferenceOriginsTargetingPos(path lang.Path, file string, pos hcl.Pos) ReferenceOrigins {
	origins := make(ReferenceOrigins, 0)

	ctx := context.Background()

	// On a reference, these are the targets of that reference,
	// not the declaration which encloses it.
	targets, _ := d.SymbolTargetsAtPos(path, file, pos)
	if len(targets) == 0 {
		return ReferenceOrigins{}
	}

	type originKey struct {
		path lang.Path
		rng  hcl.Range
	}
	seen := make(map[originKey]bool, 0)
	for _, target := range targets {
		for _, origin := range d.OriginsTargeting(ctx, target.Target, target.Path) {
			rng := origin.Origin.OriginRange()
			key := originKey{path: origin.Path, rng: rng}
			if seen[key] {
				// the same origin can match two targets for the same
				// symbol, e.g. a block targetable as a type-less
				// reference and as an object
				continue
			}
			seen[key] = true
			origins = append(origins, ReferenceOrigin{
				Path:  origin.Path,
				Range: rng,
			})
		}
	}

	sort.SliceStable(origins, func(i, j int) bool {
		if origins[i].Path.Path != origins[j].Path.Path {
			return origins[i].Path.Path < origins[j].Path.Path
		}
		if origins[i].Range.Filename != origins[j].Range.Filename {
			return origins[i].Range.Filename < origins[j].Range.Filename
		}
		return origins[i].Range.Start.Byte < origins[j].Range.Start.Byte
	})

	return origins
}

func (d *PathDecoder) CollectReferenceOrigins() (reference.Origins, error) {
	refOrigins := make(reference.Origins, 0)
	impliedOrigins := make([]schema.ImpliedOrigin, 0)

	if d.pathCtx.Schema == nil {
		// unable to collect reference origins without schema
		return refOrigins, &NoSchemaError{}
	}

	files := d.filenames()
	for _, filename := range files {
		f, err := d.fileByName(filename)
		if err != nil {
			// skip unparseable file
			continue
		}

		os, ios := d.referenceOriginsInBody(f.Body, d.pathCtx.Schema)
		refOrigins = append(refOrigins, os...)
		impliedOrigins = append(impliedOrigins, ios...)
	}

	for _, impliedOrigin := range impliedOrigins {
		for _, origin := range refOrigins {
			localOrigin, ok := origin.(reference.LocalOrigin)

			if ok && localOrigin.Address().Equals(impliedOrigin.OriginAddress) {
				refOrigins = append(refOrigins, reference.PathOrigin{
					Range:      origin.OriginRange(),
					TargetAddr: impliedOrigin.TargetAddress,
					TargetPath: impliedOrigin.Path,
					Constraints: reference.OriginConstraints{
						{
							OfScopeId: impliedOrigin.Constraints.ScopeId,
							OfType:    impliedOrigin.Constraints.Type,
						},
					},
				})
			}
		}
	}

	sort.SliceStable(refOrigins, func(i, j int) bool {
		if refOrigins[i].OriginRange().Filename != refOrigins[j].OriginRange().Filename {
			return refOrigins[i].OriginRange().Filename < refOrigins[j].OriginRange().Filename
		}
		return refOrigins[i].OriginRange().Start.Byte < refOrigins[j].OriginRange().Start.Byte
	})

	return refOrigins, nil
}

func (d *PathDecoder) referenceOriginsInBody(body hcl.Body, bodySchema *schema.BodySchema) (reference.Origins, []schema.ImpliedOrigin) {
	origins := make(reference.Origins, 0)
	impliedOrigins := make([]schema.ImpliedOrigin, 0)

	if bodySchema == nil {
		return origins, impliedOrigins
	}

	ctx := context.Background()

	impliedOrigins = append(impliedOrigins, bodySchema.ImpliedOrigins...)
	content := ast.DecodeBody(body, bodySchema)

	for _, attr := range content.Attributes {
		var aSchema *schema.AttributeSchema
		if bodySchema.Extensions != nil && bodySchema.Extensions.Count && attr.Name == "count" {
			aSchema = schemahelper.CountAttributeSchema()
		} else if bodySchema.Extensions != nil && bodySchema.Extensions.ForEach && attr.Name == "for_each" {
			aSchema = schemahelper.ForEachAttributeSchema()
		} else {
			var ok bool
			aSchema, ok = bodySchema.Attributes[attr.Name]
			if !ok {
				if bodySchema.AnyAttribute == nil {
					// skip unknown attribute
					continue
				}
				aSchema = bodySchema.AnyAttribute
			}
		}

		if aSchema.OriginForTarget != nil {
			targetAddr, ok := resolveAttributeAddress(attr, aSchema.OriginForTarget.Address)
			if ok {
				origins = append(origins, reference.PathOrigin{
					Range:      attr.NameRange,
					TargetAddr: targetAddr,
					TargetPath: aSchema.OriginForTarget.Path,
					Constraints: reference.OriginConstraints{
						{
							OfScopeId: aSchema.OriginForTarget.Constraints.ScopeId,
							OfType:    aSchema.OriginForTarget.Constraints.Type,
						},
					},
				})
			}
		}

		if aSchema.IsDepKey && bodySchema.Targets != nil {
			origins = append(origins, reference.DirectOrigin{
				Range:       attr.Expr.Range(),
				TargetPath:  bodySchema.Targets.Path,
				TargetRange: bodySchema.Targets.Range,
			})
		}

		if bodySchema.Extensions != nil && bodySchema.Extensions.SelfRefs {
			ctx = schema.WithActiveSelfRefs(ctx)
		}
		expr := d.newExpression(attr.Expr, aSchema.Constraint)
		if eType, ok := expr.(ReferenceOriginsExpression); ok {
			origins = append(origins, eType.ReferenceOrigins(ctx)...)
		}
	}

	// Collect refs from unknown attributes if extension enabled
	if bodySchema.Extensions != nil && bodySchema.Extensions.UnknownRefs {
		origins = append(origins, d.collectUnknownAttributeOrigins(ctx, body, bodySchema)...)
	}

	for _, block := range content.Blocks {
		if block.Body != nil {
			bSchema, ok := bodySchema.Blocks[block.Type]
			if !ok {
				// Handle unknown blocks if extension enabled
				if bodySchema.Extensions != nil && bodySchema.Extensions.UnknownRefs {
					unknownSchema := &schema.BodySchema{
						Extensions: &schema.BodyExtensions{UnknownRefs: true},
					}
					os, _ := d.referenceOriginsInBody(block.Body, unknownSchema)
					origins = append(origins, os...)
				}
				continue
			}
			mergedSchema, _ := schemahelper.MergeBlockBodySchemas(block.Block, bSchema)

			os, ios := d.referenceOriginsInBody(block.Body, mergedSchema)
			origins = append(origins, os...)
			impliedOrigins = append(impliedOrigins, ios...)
		}
	}

	return origins, impliedOrigins
}

// collectUnknownAttributeOrigins extracts reference origins from attributes
// not defined in the schema. Uses DynamicPseudoType since type is unknown.
func (d *PathDecoder) collectUnknownAttributeOrigins(ctx context.Context, body hcl.Body, bodySchema *schema.BodySchema) reference.Origins {
	origins := make(reference.Origins, 0)

	// Get all attributes from body
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return origins
	}

	for name, attr := range attrs {
		// Skip if attribute is in schema (already processed)
		if bodySchema.Attributes != nil {
			if _, exists := bodySchema.Attributes[name]; exists {
				continue
			}
		}
		// Skip if AnyAttribute is defined (already processed)
		if bodySchema.AnyAttribute != nil {
			continue
		}
		// Skip count/for_each if extensions enabled
		if bodySchema.Extensions != nil {
			if bodySchema.Extensions.Count && name == "count" {
				continue
			}
			if bodySchema.Extensions.ForEach && name == "for_each" {
				continue
			}
		}

		// Treat as AnyExpression with DynamicPseudoType
		expr := d.newExpression(attr.Expr, schema.AnyExpression{
			OfType: cty.DynamicPseudoType,
		})
		if refExpr, ok := expr.(ReferenceOriginsExpression); ok {
			origins = append(origins, refExpr.ReferenceOrigins(ctx)...)
		}
	}

	return origins
}
