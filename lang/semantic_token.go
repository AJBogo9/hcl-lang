// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"github.com/hashicorp/hcl/v2"
)

type SemanticToken struct {
	Type      SemanticTokenType
	Modifiers SemanticTokenModifiers
	Range     hcl.Range
}

type SemanticTokenType string

type SemanticTokenTypes []SemanticTokenType

const (
	// structural tokens
	TokenAttrName   SemanticTokenType = "hcl-attrName"
	TokenBlockType  SemanticTokenType = "hcl-blockType"
	TokenBlockLabel SemanticTokenType = "hcl-blockLabel"

	// expressions
	TokenBool          SemanticTokenType = "hcl-bool"
	TokenString        SemanticTokenType = "hcl-string"
	TokenNumber        SemanticTokenType = "hcl-number"
	TokenObjectKey     SemanticTokenType = "hcl-objectKey"
	TokenMapKey        SemanticTokenType = "hcl-mapKey"
	TokenKeyword       SemanticTokenType = "hcl-keyword"
	TokenReferenceStep SemanticTokenType = "hcl-referenceStep"
	TokenTypeComplex   SemanticTokenType = "hcl-typeComplex"
	TokenTypePrimitive SemanticTokenType = "hcl-typePrimitive"
	TokenFunctionName  SemanticTokenType = "hcl-functionName"
)

var SupportedSemanticTokenTypes = SemanticTokenTypes{
	TokenAttrName,
	TokenBlockType,
	TokenBlockLabel,
	TokenBool,
	TokenString,
	TokenNumber,
	TokenObjectKey,
	TokenMapKey,
	TokenKeyword,
	TokenReferenceStep,
	TokenTypeComplex,
	TokenTypePrimitive,
	TokenFunctionName,
}

type SemanticTokenModifier string

type SemanticTokenModifiers []SemanticTokenModifier

func (stm SemanticTokenModifiers) Copy() SemanticTokenModifiers {
	if stm == nil {
		return nil
	}

	modifiersCopy := make(SemanticTokenModifiers, len(stm))
	copy(modifiersCopy, stm)
	return modifiersCopy
}

const (
	TokenModifierDependent = SemanticTokenModifier("hcl-dependent")

	// TokenModifierMetaArgument marks an attribute or a block which controls
	// how its parent block is evaluated (e.g. count, for_each, dynamic)
	// rather than passing data to it.
	TokenModifierMetaArgument = SemanticTokenModifier("hcl-metaArgument")

	// Roles of a step within a reference (traversal), reported alongside
	// the modifiers of the reference kind (see schema.ReferenceKind).

	// TokenModifierKeywordStep marks a fixed keyword step which selects
	// the kind of the reference (e.g. var in var.name).
	TokenModifierKeywordStep = SemanticTokenModifier("hcl-keywordStep")
	// TokenModifierTypeStep marks a step naming the type of the referenced
	// object (e.g. aws_instance in aws_instance.name).
	TokenModifierTypeStep = SemanticTokenModifier("hcl-typeStep")
	// TokenModifierNameStep marks a step naming the referenced object
	// (e.g. name in var.name).
	TokenModifierNameStep = SemanticTokenModifier("hcl-nameStep")
	// TokenModifierAttrStep marks a step selecting an attribute of the
	// referenced object (e.g. id in aws_instance.name.id).
	TokenModifierAttrStep = SemanticTokenModifier("hcl-attrStep")
)

// SupportedSemanticTokenModifiers lists every modifier reported by hcl-lang
// itself (as opposed to schema-defined modifiers).
var SupportedSemanticTokenModifiers = SemanticTokenModifiers{
	TokenModifierDependent,
	TokenModifierMetaArgument,
	TokenModifierKeywordStep,
	TokenModifierTypeStep,
	TokenModifierNameStep,
	TokenModifierAttrStep,
}
