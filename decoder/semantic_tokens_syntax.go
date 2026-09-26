// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"sort"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// This file derives semantic tokens from the syntax of expressions alone.
// It is only active with PathContext.SemanticHighlighting and fills the gaps
// the schema-driven tokens leave: expressions in dynamic-typed places,
// unknown attributes and blocks, for expressions, splats, relative
// traversals and references which do not resolve. It only reports what
// the syntax proves: the kind of a reference from its first step,
// function names, object keys and the for/in/if keywords.

type iteratorScopeCtxKey struct{}

// iteratorSymbol describes the shape of references to an iterator symbol
type iteratorSymbol struct {
	// keywordSteps is 1 for a dynamic block iterator (rule.value)
	// and 0 for a for expression symbol (v.attr)
	keywordSteps int
	// nameSteps is the number of steps naming the element (value in rule.value)
	nameSteps int
}

var (
	forIteratorSymbol     = iteratorSymbol{keywordSteps: 0, nameSteps: 1}
	dynamicIteratorSymbol = iteratorSymbol{keywordSteps: 1, nameSteps: 1}
)

func withIteratorSymbol(ctx context.Context, name string, sym iteratorSymbol) context.Context {
	parent, _ := ctx.Value(iteratorScopeCtxKey{}).(map[string]iteratorSymbol)
	symbols := make(map[string]iteratorSymbol, len(parent)+1)
	for k, v := range parent {
		symbols[k] = v
	}
	symbols[name] = sym
	return context.WithValue(ctx, iteratorScopeCtxKey{}, symbols)
}

func iteratorSymbolFromContext(ctx context.Context, name string) (iteratorSymbol, bool) {
	symbols, ok := ctx.Value(iteratorScopeCtxKey{}).(map[string]iteratorSymbol)
	if !ok {
		return iteratorSymbol{}, false
	}
	sym, ok := symbols[name]
	return sym, ok
}

func (sym iteratorSymbol) kind(sh *schema.SemanticHighlighting) schema.ReferenceKind {
	return schema.ReferenceKind{
		Modifiers:    sh.IteratorModifiers,
		KeywordSteps: sym.keywordSteps,
		NameSteps:    sym.nameSteps,
	}
}

// syntaxReferenceKind returns the kind of a reference as far as it is
// known from its first step: an iterator symbol in scope, a known root
// name (e.g. var) or the default kind (e.g. a managed resource).
func (pathCtx *PathContext) syntaxReferenceKind(ctx context.Context, traversal hcl.Traversal) (schema.ReferenceKind, bool) {
	sh := pathCtx.SemanticHighlighting
	if sh == nil || len(traversal) == 0 {
		return schema.ReferenceKind{}, false
	}
	rootName := traversal.RootName()
	if sym, ok := iteratorSymbolFromContext(ctx, rootName); ok {
		return sym.kind(sh), true
	}
	return sh.KindForRootName(rootName, traversalStepCount(traversal))
}

// syntaxSemanticTokens returns tokens for expr derived from its syntax,
// or nil unless SemanticHighlighting is enabled.
func syntaxSemanticTokens(ctx context.Context, pathCtx *PathContext, expr hcl.Expression) []lang.SemanticToken {
	if pathCtx.SemanticHighlighting == nil {
		return nil
	}
	e, ok := expr.(hclsyntax.Expression)
	if !ok {
		return nil
	}
	st := syntaxTokenizer{pathCtx: pathCtx}
	return st.tokensForExpr(ctx, e, nil)
}

type syntaxTokenizer struct {
	pathCtx *PathContext
}

// anonSource carries the kind of a splat's source to the
// anonymous symbol which stands for each of its elements
type anonSource struct {
	kind  schema.ReferenceKind
	known bool
}

func (st syntaxTokenizer) tokensForExpr(ctx context.Context, expr hclsyntax.Expression, anon *anonSource) []lang.SemanticToken {
	tokens := make([]lang.SemanticToken, 0)

	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		if kind, ok := st.pathCtx.syntaxReferenceKind(ctx, e.Traversal); ok {
			tokens = append(tokens, semanticTokensForTraversal(e.Traversal, &kind)...)
		}
	case *hclsyntax.RelativeTraversalExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Source, anon)...)
		tokens = append(tokens, st.relativeStepTokens(ctx, e.Source, e.Traversal, anon)...)
	case *hclsyntax.SplatExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Source, anon)...)
		kind, known := st.sourceKind(ctx, e.Source, anon)
		tokens = append(tokens, st.tokensForExpr(ctx, e.Each, &anonSource{kind: kind, known: known})...)
	case *hclsyntax.IndexExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Collection, anon)...)
		tokens = append(tokens, st.tokensForExpr(ctx, e.Key, anon)...)
	case *hclsyntax.FunctionCallExpr:
		tokens = append(tokens, lang.SemanticToken{
			Type:      lang.TokenFunctionName,
			Modifiers: []lang.SemanticTokenModifier{},
			Range:     e.NameRange,
		})
		for _, arg := range e.Args {
			tokens = append(tokens, st.tokensForExpr(ctx, arg, anon)...)
		}
	case *hclsyntax.ObjectConsExpr:
		for _, item := range e.Items {
			tokens = append(tokens, st.tokensForObjectKey(ctx, item.KeyExpr, anon)...)
			tokens = append(tokens, st.tokensForExpr(ctx, item.ValueExpr, anon)...)
		}
	case *hclsyntax.TupleConsExpr:
		for _, elem := range e.Exprs {
			tokens = append(tokens, st.tokensForExpr(ctx, elem, anon)...)
		}
	case *hclsyntax.TemplateExpr:
		for _, part := range e.Parts {
			tokens = append(tokens, st.tokensForExpr(ctx, part, anon)...)
		}
	case *hclsyntax.TemplateWrapExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Wrapped, anon)...)
	case *hclsyntax.TemplateJoinExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Tuple, anon)...)
	case *hclsyntax.ConditionalExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Condition, anon)...)
		tokens = append(tokens, st.tokensForExpr(ctx, e.TrueResult, anon)...)
		tokens = append(tokens, st.tokensForExpr(ctx, e.FalseResult, anon)...)
	case *hclsyntax.BinaryOpExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.LHS, anon)...)
		tokens = append(tokens, st.tokensForExpr(ctx, e.RHS, anon)...)
	case *hclsyntax.UnaryOpExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Val, anon)...)
	case *hclsyntax.ParenthesesExpr:
		tokens = append(tokens, st.tokensForExpr(ctx, e.Expression, anon)...)
	case *hclsyntax.ForExpr:
		tokens = append(tokens, st.tokensForForExpr(ctx, e, anon)...)
	}

	return tokens
}

func (st syntaxTokenizer) tokensForObjectKey(ctx context.Context, expr hclsyntax.Expression, anon *anonSource) []lang.SemanticToken {
	keyExpr, ok := expr.(*hclsyntax.ObjectConsKeyExpr)
	if !ok {
		return st.tokensForExpr(ctx, expr, anon)
	}
	if !keyExpr.ForceNonLiteral {
		if trav, ok := keyExpr.Wrapped.(*hclsyntax.ScopeTraversalExpr); ok && len(trav.Traversal) == 1 {
			// a bare identifier is a literal key, not a reference
			return []lang.SemanticToken{
				{
					Type:      lang.TokenObjectKey,
					Modifiers: []lang.SemanticTokenModifier{},
					Range:     trav.Range(),
				},
			}
		}
	}
	return st.tokensForExpr(ctx, keyExpr.Wrapped, anon)
}

// sourceKind returns the kind of the reference an expression starts from,
// e.g. local for local.subnets[each.key] or aws_instance.web[*]
func (st syntaxTokenizer) sourceKind(ctx context.Context, expr hclsyntax.Expression, anon *anonSource) (schema.ReferenceKind, bool) {
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return st.pathCtx.syntaxReferenceKind(ctx, e.Traversal)
	case *hclsyntax.RelativeTraversalExpr:
		return st.sourceKind(ctx, e.Source, anon)
	case *hclsyntax.IndexExpr:
		return st.sourceKind(ctx, e.Collection, anon)
	case *hclsyntax.SplatExpr:
		return st.sourceKind(ctx, e.Source, anon)
	case *hclsyntax.ParenthesesExpr:
		return st.sourceKind(ctx, e.Expression, anon)
	case *hclsyntax.AnonSymbolExpr:
		if anon != nil {
			return anon.kind, anon.known
		}
	}
	return schema.ReferenceKind{}, false
}

// relativeStepTokens returns tokens for the steps of a relative traversal,
// which select attributes of whatever its source evaluates to.
func (st syntaxTokenizer) relativeStepTokens(ctx context.Context, source hclsyntax.Expression, traversal hcl.Traversal, anon *anonSource) []lang.SemanticToken {
	kind, ok := st.sourceKind(ctx, source, anon)
	if !ok {
		// the steps still select attributes, of an unknown kind
		kind = schema.ReferenceKind{}
	}
	// every step of a relative traversal is past the name steps
	first := kind.KeywordSteps + kind.TypeSteps + kind.NameSteps
	return semanticTokensForTraversalFrom(traversal, &kind, first)
}

func (st syntaxTokenizer) tokensForForExpr(ctx context.Context, e *hclsyntax.ForExpr, anon *anonSource) []lang.SemanticToken {
	sh := st.pathCtx.SemanticHighlighting
	tokens := make([]lang.SemanticToken, 0)

	// the collection is evaluated outside of the iterator scope
	tokens = append(tokens, st.tokensForExpr(ctx, e.CollExpr, anon)...)

	innerCtx := forExprScope(ctx, e)
	if e.KeyExpr != nil {
		tokens = append(tokens, st.tokensForExpr(innerCtx, e.KeyExpr, anon)...)
	}
	tokens = append(tokens, st.tokensForExpr(innerCtx, e.ValExpr, anon)...)
	if e.CondExpr != nil {
		tokens = append(tokens, st.tokensForExpr(innerCtx, e.CondExpr, anon)...)
	}

	// The keywords and the iterator symbol declarations have no ranges
	// in the AST, so we find them by lexing whatever lies between
	// the sub-expressions.
	f, ok := st.pathCtx.Files[e.SrcRange.Filename]
	if !ok || e.SrcRange.End.Byte > len(f.Bytes) || e.SrcRange.Start.Byte >= e.SrcRange.End.Byte {
		return tokens
	}
	subExprs := []hcl.Range{e.CollExpr.Range(), e.ValExpr.Range()}
	if e.KeyExpr != nil {
		subExprs = append(subExprs, e.KeyExpr.Range())
	}
	if e.CondExpr != nil {
		subExprs = append(subExprs, e.CondExpr.Range())
	}
	src := f.Bytes[e.SrcRange.Start.Byte:e.SrcRange.End.Byte]
	lexTokens, _ := hclsyntax.LexExpression(src, e.SrcRange.Filename, e.SrcRange.Start)

	declaring := false
	for _, lt := range lexTokens {
		if lt.Type != hclsyntax.TokenIdent || rangeWithinAny(lt.Range, subExprs) {
			continue
		}
		switch name := string(lt.Bytes); {
		case name == "for" || name == "in" || name == "if":
			tokens = append(tokens, lang.SemanticToken{
				Type:      lang.TokenKeyword,
				Modifiers: []lang.SemanticTokenModifier{},
				Range:     lt.Range,
			})
			declaring = name == "for"
		case declaring && (name == e.KeyVar || name == e.ValVar):
			modifiers := make([]lang.SemanticTokenModifier, 0, len(sh.IteratorModifiers)+1)
			modifiers = append(modifiers, sh.IteratorModifiers...)
			modifiers = append(modifiers, lang.TokenModifierNameStep)
			tokens = append(tokens, lang.SemanticToken{
				Type:      lang.TokenReferenceStep,
				Modifiers: modifiers,
				Range:     lt.Range,
			})
		}
	}

	return tokens
}

// forExprScope puts the iterator symbols a for expression declares
// in scope for its key, value and condition expressions
func forExprScope(ctx context.Context, e *hclsyntax.ForExpr) context.Context {
	if e.KeyVar != "" {
		ctx = withIteratorSymbol(ctx, e.KeyVar, forIteratorSymbol)
	}
	if e.ValVar != "" {
		ctx = withIteratorSymbol(ctx, e.ValVar, forIteratorSymbol)
	}
	return ctx
}

func rangeWithinAny(rng hcl.Range, ranges []hcl.Range) bool {
	for _, r := range ranges {
		if rng.Start.Byte >= r.Start.Byte && rng.End.Byte <= r.End.Byte {
			return true
		}
	}
	return false
}

// syntaxTokensForBody returns tokens derived from syntax for every
// expression in a body the schema does not describe.
func (d *PathDecoder) syntaxTokensForBody(ctx context.Context, body *hclsyntax.Body) []lang.SemanticToken {
	tokens := make([]lang.SemanticToken, 0)
	if d.pathCtx.SemanticHighlighting == nil || body == nil {
		return tokens
	}

	dynamicBody, _ := ctx.Value(dynamicBodyCtxKey{}).(*hclsyntax.Body)
	for name, attr := range body.Attributes {
		if dynamicBody == body && name == "iterator" {
			// reported by dynamicBlockIterator
			continue
		}
		tokens = append(tokens, syntaxSemanticTokens(ctx, d.pathCtx, attr.Expr)...)
	}
	for _, block := range body.Blocks {
		blockCtx := ctx
		if block.Type == "dynamic" {
			var iterTokens []lang.SemanticToken
			blockCtx, iterTokens = d.dynamicBlockIterator(ctx, block)
			tokens = append(tokens, iterTokens...)
			blockCtx = context.WithValue(blockCtx, dynamicBodyCtxKey{}, block.Body)
		}
		tokens = append(tokens, d.syntaxTokensForBody(blockCtx, block.Body)...)
	}

	return tokens
}

// dynamicBlockIterator puts the iterator symbol of a dynamic block
// (its label, or the iterator attribute) in scope for its body and
// returns a token for the iterator attribute's value, if any.
func (d *PathDecoder) dynamicBlockIterator(ctx context.Context, block *hclsyntax.Block) (context.Context, []lang.SemanticToken) {
	sh := d.pathCtx.SemanticHighlighting
	if sh == nil || block.Body == nil {
		return ctx, nil
	}

	name := ""
	if len(block.Labels) == 1 {
		name = block.Labels[0]
	}
	var tokens []lang.SemanticToken
	if attr, ok := block.Body.Attributes["iterator"]; ok {
		if trav, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr); ok && len(trav.Traversal) == 1 {
			name = trav.Traversal.RootName()
			kind := dynamicIteratorSymbol.kind(sh)
			tokens = semanticTokensForTraversal(trav.Traversal, &kind)
		}
	}
	if name == "" {
		return ctx, tokens
	}
	return withIteratorSymbol(ctx, name, dynamicIteratorSymbol), tokens
}

// mergeSyntaxTokens adds the syntax tokens which do not overlap
// any of the schema-driven tokens, which always take precedence.
func mergeSyntaxTokens(tokens, syntaxTokens []lang.SemanticToken) []lang.SemanticToken {
	if len(syntaxTokens) == 0 {
		return tokens
	}

	// the byte ranges covered by schema-driven tokens, as disjoint sorted intervals
	type interval struct{ start, end int }
	covered := make([]interval, 0, len(tokens))
	for _, t := range tokens {
		covered = append(covered, interval{t.Range.Start.Byte, t.Range.End.Byte})
	}
	sort.Slice(covered, func(i, j int) bool {
		return covered[i].start < covered[j].start
	})
	merged := covered[:0]
	for _, c := range covered {
		if n := len(merged); n > 0 && c.start <= merged[n-1].end {
			if c.end > merged[n-1].end {
				merged[n-1].end = c.end
			}
			continue
		}
		merged = append(merged, c)
	}

	for _, st := range syntaxTokens {
		// the first interval ending after the token starts is the only candidate
		i := sort.Search(len(merged), func(i int) bool {
			return merged[i].end > st.Range.Start.Byte
		})
		if i < len(merged) && merged[i].start < st.Range.End.Byte {
			continue
		}
		tokens = append(tokens, st)
	}
	return tokens
}

// templateHasReferences reports whether expr is a string template which
// interpolates references, such as an error_message of
// "Got ${var.cfg.name}.": OpenTofu evaluates it although the schema
// describes a literal string.
func templateHasReferences(expr hcl.Expression) bool {
	switch expr.(type) {
	case *hclsyntax.TemplateExpr, *hclsyntax.TemplateWrapExpr:
		return len(expr.Variables()) > 0
	}
	return false
}

// constraintAllowsReferences reports whether an expression of the given
// constraint may contain references, i.e. whether tokens derived from
// syntax may be reported for it. Type declarations, keywords and literal
// values are fully described by their schema-driven tokens.
func constraintAllowsReferences(cons schema.Constraint) bool {
	switch c := cons.(type) {
	case schema.AnyExpression, schema.Reference:
		return true
	case schema.List:
		return c.Elem != nil && constraintAllowsReferences(c.Elem)
	case schema.Set:
		return c.Elem != nil && constraintAllowsReferences(c.Elem)
	case schema.Map:
		return c.Elem != nil && constraintAllowsReferences(c.Elem)
	case schema.Tuple:
		for _, elem := range c.Elems {
			if constraintAllowsReferences(elem) {
				return true
			}
		}
	case schema.Object:
		for _, attr := range c.Attributes {
			if attr != nil && constraintAllowsReferences(attr.Constraint) {
				return true
			}
		}
	case schema.OneOf:
		for _, oc := range c {
			if constraintAllowsReferences(oc) {
				return true
			}
		}
	}
	return false
}
