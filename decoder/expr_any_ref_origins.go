// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"

	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func (a Any) ReferenceOrigins(ctx context.Context) reference.Origins {
	typ := a.cons.OfType

	if typ.IsListType() {
		_, ok := a.expr.(*hclsyntax.TupleConsExpr)
		if !ok {
			return a.refOriginsForNonComplexExpr(ctx)
		}

		list := List{
			expr:    a.expr,
			pathCtx: a.pathCtx,
			cons: schema.List{
				Elem: schema.AnyExpression{
					OfType: typ.ElementType(),
				},
			},
		}
		return list.ReferenceOrigins(ctx)
	}

	if typ.IsSetType() {
		_, ok := a.expr.(*hclsyntax.TupleConsExpr)
		if !ok {
			return a.refOriginsForNonComplexExpr(ctx)
		}

		set := Set{
			expr:    a.expr,
			pathCtx: a.pathCtx,
			cons: schema.Set{
				Elem: schema.AnyExpression{
					OfType: typ.ElementType(),
				},
			},
		}
		return set.ReferenceOrigins(ctx)
	}

	if typ.IsTupleType() {
		_, ok := a.expr.(*hclsyntax.TupleConsExpr)
		if !ok {
			return a.refOriginsForNonComplexExpr(ctx)
		}

		elemTypes := typ.TupleElementTypes()
		cons := schema.Tuple{
			Elems: make([]schema.Constraint, len(elemTypes)),
		}
		for i, elemType := range elemTypes {
			cons.Elems[i] = schema.LiteralType{
				Type: elemType,
			}
		}

		tuple := Tuple{
			expr:    a.expr,
			pathCtx: a.pathCtx,
			cons:    cons,
		}
		return tuple.ReferenceOrigins(ctx)
	}

	if typ.IsMapType() {
		_, ok := a.expr.(*hclsyntax.ObjectConsExpr)
		if !ok {
			return a.refOriginsForNonComplexExpr(ctx)
		}

		m := Map{
			expr:    a.expr,
			pathCtx: a.pathCtx,
			cons: schema.Map{
				Elem: schema.AnyExpression{
					OfType: typ.ElementType(),
				},
				AllowInterpolatedKeys: true,
			},
		}
		return m.ReferenceOrigins(ctx)
	}

	if typ.IsObjectType() {
		_, ok := a.expr.(*hclsyntax.ObjectConsExpr)
		if !ok {
			return a.refOriginsForNonComplexExpr(ctx)
		}

		obj := Object{
			expr:    a.expr,
			pathCtx: a.pathCtx,
			cons: schema.Object{
				Attributes:            ctyObjectToObjectAttributes(typ),
				AllowInterpolatedKeys: true,
			},
		}
		return obj.ReferenceOrigins(ctx)
	}

	return a.refOriginsForNonComplexExpr(ctx)
}

func (a Any) refOriginsForNonComplexExpr(ctx context.Context) reference.Origins {
	// TODO: Support splat expression https://github.com/hashicorp/terraform-ls/issues/526
	// TODO: Support relative traversals https://github.com/hashicorp/terraform-ls/issues/532

	if origins, ok := a.refOriginsForOperatorExpr(ctx); ok {
		return origins
	}

	if origins, ok := a.refOriginsForTemplateExpr(ctx); ok {
		return origins
	}

	if origins, ok := a.refOriginsForConditionalExpr(ctx); ok {
		return origins
	}

	if origins, ok := a.refOriginsForForExpr(ctx); ok {
		return origins
	}

	// attempt to get accurate constraint for the origins
	// if we recognise the given expression
	funcExpr := functionExpr{
		expr:       a.expr,
		returnType: a.cons.OfType,
		pathCtx:    a.pathCtx,
	}
	origins := funcExpr.ReferenceOrigins(ctx)
	if len(origins) > 0 {
		return origins
	}

	// If we're dealing with a valid function call expression that doesn't contain
	// any origins, there is no more work todo here and nothing below would match,
	// so we can return early.
	_, diags := hcl.ExprCall(a.expr)
	if !diags.HasErrors() {
		return origins
	}

	allowSelfRefs := schema.ActiveSelfRefsFromContext(ctx)
	if origins, ok := a.refOriginsForScopedIndexExpr(ctx); ok {
		return origins
	}
	te, ok := a.expr.(*hclsyntax.ScopeTraversalExpr)
	if ok {
		oCons := reference.OriginConstraints{
			{OfType: a.cons.OfType, OfScopeId: a.cons.OfScopeId},
		}
		origin, ok := reference.TraversalToLocalOrigin(te.Traversal, oCons, allowSelfRefs)
		if ok {
			return reference.Origins{origin}
		}

		return reference.Origins{}
	}

	if origins, ok := a.refOriginsForInstanceTraversal(ctx); ok {
		return origins
	}

	// if not we just collect any/all origins with vague constraint
	// as that is safest
	origins = make(reference.Origins, 0)
	vars := a.expr.Variables()
	for _, traversal := range vars {
		oCons := reference.OriginConstraints{
			{OfType: cty.DynamicPseudoType},
		}
		origin, ok := reference.TraversalToLocalOrigin(traversal, oCons, allowSelfRefs)
		if ok {
			origins = append(origins, origin)
		}
	}
	return origins
}

// refOriginsForInstanceTraversal collects the origin of a splat over the
// instances of a module call, resource or data source, such as
// module.app[*].url, or of an instance picked by a dynamic key, such as
// module.app[var.key].url. The syntax splits such a traversal in two, so
// that only module.app would be an origin. Here the whole traversal is
// one origin, with an unknown key in place of the splat or the dynamic
// key, which matches module.app.url (see reference.LocalOrigin.Address)
// as module.app[0].url does. The key's own origins are collected too.
func (a Any) refOriginsForInstanceTraversal(ctx context.Context) (reference.Origins, bool) {
	var src *hclsyntax.ScopeTraversalExpr
	var rel hcl.Traversal
	var key hclsyntax.Expression
	var keyRange hcl.Range

	switch eType := a.expr.(type) {
	case *hclsyntax.SplatExpr:
		s, ok := eType.Source.(*hclsyntax.ScopeTraversalExpr)
		if !ok {
			return nil, false
		}
		each, ok := eType.Each.(*hclsyntax.RelativeTraversalExpr)
		if !ok {
			return nil, false
		}
		if _, ok := each.Source.(*hclsyntax.AnonSymbolExpr); !ok {
			return nil, false
		}
		src, rel, keyRange = s, each.Traversal, eType.MarkerRange
	case *hclsyntax.RelativeTraversalExpr:
		idx, ok := eType.Source.(*hclsyntax.IndexExpr)
		if !ok {
			return nil, false
		}
		s, ok := idx.Collection.(*hclsyntax.ScopeTraversalExpr)
		if !ok {
			return nil, false
		}
		src, rel, key, keyRange = s, eType.Traversal, idx.Key, idx.BracketRange
	default:
		return nil, false
	}
	if len(rel) == 0 {
		return nil, false
	}
	if _, ok := rel[0].(hcl.TraverseAttr); !ok {
		return nil, false
	}

	traversal := make(hcl.Traversal, 0, len(src.Traversal)+1+len(rel))
	traversal = append(traversal, src.Traversal...)
	traversal = append(traversal, hcl.TraverseIndex{Key: cty.DynamicVal, SrcRange: keyRange})
	traversal = append(traversal, rel...)

	allowSelfRefs := schema.ActiveSelfRefsFromContext(ctx)
	origin, ok := reference.TraversalToLocalOrigin(traversal, reference.OriginConstraints{
		{OfType: cty.DynamicPseudoType},
	}, allowSelfRefs)
	if !ok {
		return nil, false
	}
	if len(origin.Address()) == len(origin.Addr) {
		// the index does not pick an instance, e.g. var.list[*].name
		// or aws_instance.web.tags[var.k].x, whose origin is the value
		// before the index, as collected below
		return nil, false
	}

	origins := reference.Origins{origin}
	if key != nil {
		// as the fallback below collects them
		for _, keyTraversal := range key.Variables() {
			keyOrigin, ok := reference.TraversalToLocalOrigin(keyTraversal, reference.OriginConstraints{
				{OfType: cty.DynamicPseudoType},
			}, allowSelfRefs)
			if ok {
				origins = append(origins, keyOrigin)
			}
		}
	}
	return origins, true
}

// refOriginsForScopedIndexExpr collects the origins of an expression of
// a scope (see schema.AnyExpression.OfScopeId) which picks an instance
// by a key, such as random.by_key[var.k] naming a provider: the traversal
// is of the scope, the key's references are any values.
func (a Any) refOriginsForScopedIndexExpr(ctx context.Context) (reference.Origins, bool) {
	if a.cons.OfScopeId == "" {
		return nil, false
	}
	idx, ok := a.expr.(*hclsyntax.IndexExpr)
	if !ok {
		return nil, false
	}
	coll, ok := idx.Collection.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return nil, false
	}
	allowSelfRefs := schema.ActiveSelfRefsFromContext(ctx)
	origins := make(reference.Origins, 0)
	origin, ok := reference.TraversalToLocalOrigin(coll.Traversal, reference.OriginConstraints{
		{OfType: cty.DynamicPseudoType, OfScopeId: a.cons.OfScopeId},
	}, allowSelfRefs)
	if ok {
		origins = append(origins, origin)
	}
	for _, keyTraversal := range idx.Key.Variables() {
		keyOrigin, ok := reference.TraversalToLocalOrigin(keyTraversal, reference.OriginConstraints{
			{OfType: cty.DynamicPseudoType},
		}, allowSelfRefs)
		if ok {
			origins = append(origins, keyOrigin)
		}
	}
	return origins, true
}
