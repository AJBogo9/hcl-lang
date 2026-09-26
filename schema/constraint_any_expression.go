// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"context"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/zclconf/go-cty/cty"
)

// AnyExpression represents any expression type convertible
// to the given data type (OfType).
//
// For example function call returning cty.String complies with
// AnyExpression{OfType: cty.String}.
type AnyExpression struct {
	// OfType defines the type which the outermost expression is constrained to
	OfType cty.Type

	// SkipLiteralComplexTypes avoids descending into complex literal types, such as {} and [].
	// It might be required when AnyExpression is used in OneOf to avoid duplicates.
	SkipLiteralComplexTypes bool

	// OfScopeId, when set, is the scope of the reference which the
	// expression is: a traversal, or a traversal with an index whose key
	// is any expression, such as random.by_key[var.k].
	OfScopeId lang.ScopeId
}

func (AnyExpression) isConstraintImpl() constraintSigil {
	return constraintSigil{}
}

func (ae AnyExpression) FriendlyName() string {
	return ae.OfType.FriendlyNameForConstraint()
}

func (ae AnyExpression) Copy() Constraint {
	return AnyExpression{
		OfType:                  ae.OfType,
		SkipLiteralComplexTypes: ae.SkipLiteralComplexTypes,
		OfScopeId:               ae.OfScopeId,
	}
}

func (ae AnyExpression) EmptyCompletionData(ctx context.Context, nextPlaceholder int, nestingLevel int) CompletionData {
	// if prefilling is enabled, then it is desirable to treat this as literal
	if prefillRequiredFields(ctx) {
		lt := LiteralType{
			Type:             ae.OfType,
			SkipComplexTypes: ae.SkipLiteralComplexTypes,
		}
		return lt.EmptyCompletionData(ctx, nextPlaceholder, nestingLevel)
	}

	// otherwise (which can just be when completing attribute)
	// we assume the user will more likely want reference-like behaviour
	ref := Reference{
		OfType: ae.OfType,
	}
	return ref.EmptyCompletionData(ctx, nextPlaceholder, nestingLevel)
}

func (ae AnyExpression) ConstraintType() (cty.Type, bool) {
	return ae.OfType, true
}
