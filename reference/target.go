// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"context"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

type Target struct {
	// Addr represents the address of the target, as available
	// elsewhere in the configuration
	Addr lang.Address

	// LocalAddr represents the address of the target
	// as available *locally* (e.g. self.attr_name)
	LocalAddr lang.Address

	// TargetableFromRangePtr defines where the target is targetable from.
	// This is considered when matching the target against origin.
	//
	// e.g. count.index is only available within the body of the block
	// where count is declared (and extension enabled)
	TargetableFromRangePtr *hcl.Range

	// TargetableFromRanges, when not empty, restricts matching to origins
	// within one of the ranges, by either address. It lets one path hold
	// targets that are in scope only in parts of a file, e.g. the objects
	// of the module which a run block of a test file runs.
	TargetableFromRanges []hcl.Range

	// ScopeId provides scope for matching/filtering
	// (in addition to Type & Addr/LocalAddr).
	//
	// There should never be two targets with the same Type & address,
	// but there are contexts (e.g. completion) where we don't filter
	// by address and may not have type either (e.g. because targets
	// are type-unaware).
	ScopeId lang.ScopeId

	// RangePtr represents range of the whole attribute or block
	// or nil if the target is not addressable.
	RangePtr *hcl.Range

	// DefRangePtr represents a definition range, i.e. block header,
	// or an attribute name or nil if the target is not addressable
	// or when it represents multiple list, set or map blocks.
	//
	// This is useful in situation where a representative single-line
	// range is needed - e.g. to render a contextual UI element in
	// the editor near the middle of this range.
	DefRangePtr *hcl.Range

	Type        cty.Type
	Name        string
	Description lang.MarkupContent

	NestedTargets Targets

	// ScopedOnly makes the target match only origins constrained to its
	// ScopeId (see schema.BlockAddrSchema.ScopedOriginsOnly).
	ScopedOnly bool
}

// rangeOverlaps is a copy of hcl.Range.Overlaps
// https://github.com/hashicorp/hcl/blob/v2.14.1/pos.go#L195-L212
// which accounts for empty ranges that are common in the context of LS
func rangeOverlaps(one, other hcl.Range) bool {
	switch {
	case one.Filename != other.Filename:
		// If the ranges are in different files then they can't possibly overlap
		return false
	case one.Empty() && other.Empty():
		// Empty ranges can never overlap
		return false
	case one.ContainsOffset(other.Start.Byte) || one.ContainsOffset(other.End.Byte):
		return true
	case other.ContainsOffset(one.Start.Byte) || other.ContainsOffset(one.End.Byte):
		return true
	default:
		return false
	}
}

func (ref Target) Copy() Target {
	return Target{
		Addr:                   ref.Addr,
		LocalAddr:              ref.LocalAddr,
		TargetableFromRangePtr: copyHclRangePtr(ref.TargetableFromRangePtr),
		TargetableFromRanges:   copyHclRanges(ref.TargetableFromRanges),
		ScopeId:                ref.ScopeId,
		RangePtr:               copyHclRangePtr(ref.RangePtr),
		DefRangePtr:            copyHclRangePtr(ref.DefRangePtr),
		Type:                   ref.Type, // cty.Type is immutable by design
		Name:                   ref.Name,
		Description:            ref.Description,
		NestedTargets:          ref.NestedTargets.Copy(),
		ScopedOnly:             ref.ScopedOnly,
	}
}

func copyHclRanges(rngs []hcl.Range) []hcl.Range {
	if rngs == nil {
		return nil
	}
	return append([]hcl.Range{}, rngs...)
}

// targetableFrom reports whether an origin in rng may match the target,
// as TargetableFromRanges restricts it.
func (target Target) targetableFrom(rng hcl.Range) bool {
	if len(target.TargetableFromRanges) == 0 {
		return true
	}
	for _, from := range target.TargetableFromRanges {
		if rangeOverlaps(from, rng) {
			return true
		}
	}
	return false
}

func copyHclRangePtr(rng *hcl.Range) *hcl.Range {
	if rng == nil {
		return nil
	}
	return rng.Ptr()
}

// Address returns any of the two non-empty addresses
// depending on the provided context
func (r Target) Address(ctx context.Context, pos hcl.Pos) lang.Address {
	if len(r.LocalAddr) > 0 {
		// If the target has only local address, use it
		if len(r.Addr) == 0 {
			return r.LocalAddr
		}

		// If the target has local self address & self is active
		if r.LocalAddr[0].String() == "self" && schema.ActiveSelfRefsFromContext(ctx) {
			// and we targeting it from the expected range
			if r.TargetableFromRangePtr != nil && r.TargetableFromRangePtr.ContainsPos(pos) {
				return r.LocalAddr
			}
		}
	}

	return r.Addr
}

func (r Target) FriendlyName() string {
	if r.Name != "" {
		return r.Name
	}

	if r.Type != cty.NilType {
		return r.Type.FriendlyName()
	}

	return "reference"
}

func (r Target) TargetRange() (hcl.Range, bool) {
	if r.RangePtr == nil {
		return hcl.Range{}, false
	}

	return *r.RangePtr, true
}

func (target Target) MatchesConstraint(ref schema.Reference) bool {
	return target.MatchesScopeId(ref.OfScopeId) && target.IsConvertibleToType(ref.OfType)
}

func (ref Target) MatchesScopeId(scopeId lang.ScopeId) bool {
	return scopeId == "" || ref.ScopeId == scopeId
}

func (ref Target) IsConvertibleToType(typ cty.Type) bool {
	isConvertible := false
	if typ != cty.NilType && ref.Type != cty.NilType {
		if ref.Type == cty.DynamicPseudoType {
			// anything is convertible to dynamic
			isConvertible = true
		}
		if _, err := convert.Convert(cty.UnknownVal(ref.Type), typ); err == nil {
			isConvertible = true
		}
	}

	return isConvertible || (typ == cty.NilType && ref.Type == cty.NilType)
}

func (target Target) Matches(origin MatchableOrigin) bool {
	if target.ScopedOnly && !constrainedToScope(origin.OriginConstraints(), target.ScopeId) {
		return false
	}
	if !target.targetableFrom(origin.OriginRange()) {
		return false
	}
	addr := origin.Address()
	if target.matchesElementOf(addr, origin.OriginConstraints()) {
		return true
	}

	originAddr, localOriginAddr := addr, addr

	matchesCons := false

	// Unconstrained origins should be uncommon, but they match any target
	if len(origin.OriginConstraints()) == 0 {
		// As long as the target is type-aware. Type-unaware targets
		// generally don't have Type, so we avoid false positive here.
		if target.Type != cty.NilType {
			matchesCons = true
		}
	}

	for _, cons := range origin.OriginConstraints() {
		if !target.MatchesScopeId(cons.OfScopeId) {
			continue
		}

		if target.Type == cty.DynamicPseudoType {
			// Account for the case where the origin address points to a nested
			// segment, which the target address doesn't explicitly contain
			// but implies.
			// e.g. If self.foo target is of "any type" (cty.DynamicPseudoType),
			// then we assume it is a match for self.foo.anything
			// by ignoring the last "anything" segment.
			if len(target.Addr) < len(origin.Address()) {
				originAddr = origin.Address().FirstSteps(uint(len(target.Addr)))
			}
			if len(target.LocalAddr) < len(origin.Address()) {
				localOriginAddr = origin.Address().FirstSteps(uint(len(target.LocalAddr)))
			}
			matchesCons = true
			continue
		}
		if cons.OfType.IsTupleType() && cons.OfType.Length() == 0 && target.Type.IsTupleType() {
			// This is a special case where we match an empty tuple (cty.EmptyTuple)
			// against any tuple.
			matchesCons = true
			continue
		}
		if cons.OfType != cty.NilType && target.IsConvertibleToType(cons.OfType) {
			matchesCons = true
		}
		if cons.OfType == cty.NilType && target.Type == cty.NilType {
			// This just simplifies testing
			matchesCons = true
		}
	}

	// If the target is only targetable from a particular range
	// we confirm that the origin is within that range.
	targetRangeMatches := true
	if target.TargetableFromRangePtr != nil && !rangeOverlaps(*target.TargetableFromRangePtr, origin.OriginRange()) {
		targetRangeMatches = false
	}

	return ((target.LocalAddr.Equals(localOriginAddr) && targetRangeMatches) || target.Addr.Equals(originAddr)) && matchesCons
}

// matchedAddrLen returns how many leading steps of the origin address
// the target's address (or local address) covers. It equals the length
// of the origin address for an exact match and is shorter when a
// type-unaware target matched a longer address under it.
func (target Target) matchedAddrLen(origin MatchableOrigin) int {
	originAddr := origin.Address()
	n := 0
	if l := len(target.Addr); l > 0 && l <= len(originAddr) && target.Addr.Equals(originAddr.FirstSteps(uint(l))) {
		n = l
	}
	if l := len(target.LocalAddr); l > n && l <= len(originAddr) && target.LocalAddr.Equals(originAddr.FirstSteps(uint(l))) {
		n = l
	}
	return n
}

// matchesElementOf reports whether the origin reads an element of the
// target through an index, such as var.names[0] (or var.subnets[0].id)
// for a variable of type list(object), which the address alone does not
// match unless the target is type-unaware. The origin's type constraint
// applies to the element, not to the target, so only its scope is
// checked.
func (target Target) matchesElementOf(addr lang.Address, cons OriginConstraints) bool {
	n := len(target.Addr)
	if n == 0 || len(addr) <= n || !isIndexableType(target.Type) {
		return false
	}
	if _, ok := addr[n].(lang.IndexStep); !ok {
		return false
	}
	if !target.Addr.Equals(addr.FirstSteps(uint(n))) {
		return false
	}
	if len(cons) == 0 {
		return true
	}
	for _, c := range cons {
		if target.MatchesScopeId(c.OfScopeId) {
			return true
		}
	}
	return false
}

// isIndexableType reports whether a value of the type has elements
// that an index step reads: a list, map, tuple or object.
func isIndexableType(typ cty.Type) bool {
	if typ == cty.NilType || typ == cty.DynamicPseudoType {
		return false
	}
	return typ.IsListType() || typ.IsMapType() || typ.IsTupleType() || typ.IsObjectType()
}

// constrainedToScope reports whether one of the constraints names the
// scope.
func constrainedToScope(cons OriginConstraints, scopeId lang.ScopeId) bool {
	for _, c := range cons {
		if c.OfScopeId == scopeId {
			return true
		}
	}
	return false
}
