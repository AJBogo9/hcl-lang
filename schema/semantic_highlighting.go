// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"github.com/hashicorp/hcl-lang/lang"
)

// SemanticHighlighting opts a path into richer semantic tokens:
//
//   - every step of a reference carries the modifiers of its kind
//     (e.g. variable, resource) plus one role modifier
//     (lang.TokenModifierKeywordStep, TypeStep, NameStep or AttrStep),
//   - references whose kind is known from syntax alone are reported
//     even when they do not resolve to a target,
//   - expressions the schema does not describe (unknown attributes,
//     dynamic-typed object constructors, for expressions, splats, ...)
//     are tokenized from their syntax,
//   - extension meta-arguments (count, for_each, dynamic) carry
//     lang.TokenModifierMetaArgument.
//
// A nil *SemanticHighlighting keeps the original behavior.
type SemanticHighlighting struct {
	// RootNames maps the name of the first step of a reference
	// (e.g. "var") to its kind.
	RootNames map[string]ReferenceKind

	// Scopes maps a reference target scope to a kind. It takes precedence
	// over RootNames when the reference is constrained to that scope,
	// or when every target it resolves to is in that scope
	// (e.g. a provider reference such as local.alias).
	Scopes map[lang.ScopeId]ReferenceKind

	// Default is the kind of any other reference with at least
	// two steps (e.g. a managed resource: aws_instance.name).
	Default *ReferenceKind

	// IteratorModifiers are reported on references to iterator symbols
	// declared by for expressions and dynamic blocks.
	IteratorModifiers lang.SemanticTokenModifiers
}

// ReferenceKind describes one kind of reference for highlighting:
// which modifiers every step carries and how the leading steps
// divide into keywords, types and names. Any step after those
// selects an attribute.
type ReferenceKind struct {
	// Modifiers are reported on every step of the reference.
	Modifiers lang.SemanticTokenModifiers

	// KeywordSteps is the number of leading fixed keyword steps
	// (1 for var.name, 0 for aws_instance.name).
	KeywordSteps int

	// TypeSteps is the number of steps after the keywords which name
	// a type (1 for data.aws_ami.name and aws_instance.name).
	TypeSteps int

	// NameSteps is the number of steps after the types which name
	// the referenced object (1 for var.name, 0 for self.attr).
	NameSteps int
}

// StepRole returns the role modifier of the i-th (0-based) attribute-like
// step of a reference of this kind. Index steps are not counted.
func (rk ReferenceKind) StepRole(i int) lang.SemanticTokenModifier {
	switch {
	case i < rk.KeywordSteps:
		return lang.TokenModifierKeywordStep
	case i < rk.KeywordSteps+rk.TypeSteps:
		return lang.TokenModifierTypeStep
	case i < rk.KeywordSteps+rk.TypeSteps+rk.NameSteps:
		return lang.TokenModifierNameStep
	}
	return lang.TokenModifierAttrStep
}

// KindForRootName returns the kind of a reference whose first step is
// rootName and which has stepCount attribute-like steps in total.
func (sh *SemanticHighlighting) KindForRootName(rootName string, stepCount int) (ReferenceKind, bool) {
	if sh == nil {
		return ReferenceKind{}, false
	}
	if kind, ok := sh.RootNames[rootName]; ok {
		return kind, true
	}
	if sh.Default != nil && stepCount >= 2 {
		return *sh.Default, true
	}
	return ReferenceKind{}, false
}
