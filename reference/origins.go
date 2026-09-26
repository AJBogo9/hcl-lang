// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl/v2"
)

type Origins []Origin

func (ro Origins) Copy() Origins {
	if ro == nil {
		return nil
	}

	newOrigins := make(Origins, len(ro))
	for i, origin := range ro {
		newOrigins[i] = origin.Copy()
	}

	return newOrigins
}

// AtPos returns the innermost origins at pos. An origin can hold another
// one: module.app[var.key].url holds var.key, and on var.key only
// var.key is the reference under the cursor.
func (ro Origins) AtPos(file string, pos hcl.Pos) (Origins, bool) {
	matchingOrigins := make(Origins, 0)
	for _, origin := range ro {
		if origin.OriginRange().Filename == file && origin.OriginRange().ContainsPos(pos) {
			matchingOrigins = append(matchingOrigins, origin)
		}
	}

	innermost := make(Origins, 0, len(matchingOrigins))
	for _, origin := range matchingOrigins {
		if !holdsAnother(origin.OriginRange(), matchingOrigins) {
			innermost = append(innermost, origin)
		}
	}

	return innermost, len(innermost) > 0
}

// holdsAnother reports whether rng strictly contains the range of one
// of the origins.
func holdsAnother(rng hcl.Range, origins Origins) bool {
	for _, other := range origins {
		o := other.OriginRange()
		if o.Start.Byte >= rng.Start.Byte && o.End.Byte <= rng.End.Byte &&
			(o.Start.Byte > rng.Start.Byte || o.End.Byte < rng.End.Byte) {
			return true
		}
	}
	return false
}

func (ro Origins) Match(localPath lang.Path, target Target, targetPath lang.Path) Origins {
	origins := make(Origins, 0)

	for _, refOrigin := range ro {
		switch origin := refOrigin.(type) {
		case LocalOrigin:
			if localPath.Equals(targetPath) && target.Matches(origin) {
				origins = append(origins, refOrigin)
			}
		case PathOrigin:
			if origin.TargetPath.Equals(targetPath) && target.Matches(origin) {
				origins = append(origins, refOrigin)
			}
		}
	}

	for _, iTarget := range target.NestedTargets {
		origins = append(origins, ro.Match(localPath, iTarget, targetPath)...)
	}

	return origins
}

// MatchResolved is like Match, but it leaves out origins which resolve
// to a more specific target among allTargets (the targets of targetPath)
// and only matched this target because it is type-unaware.
// allTargets is only called when such an origin is found.
//
// For example provider "local" {} is a dynamic target which matches
// local.name_prefix, which resolves to the local value instead.
func (ro Origins) MatchResolved(localPath lang.Path, target Target, targetPath lang.Path, allTargets func() Targets) Origins {
	origins := make(Origins, 0)

	for _, refOrigin := range ro.Match(localPath, target, targetPath) {
		if !originResolvesTo(refOrigin, target, allTargets) {
			continue
		}
		origins = append(origins, refOrigin)
	}

	return origins
}

// originResolvesTo reports whether the origin (already known to match
// target or one of its nested targets) resolves to it, i.e. no other
// target matches the origin more specifically.
func originResolvesTo(origin Origin, target Target, allTargets func() Targets) bool {
	mo, ok := origin.(MatchableOrigin)
	if !ok {
		return true
	}
	originAddr := mo.Address()
	n := 0
	var walk func(t Target)
	walk = func(t Target) {
		if !addrCouldPrefix(t.Addr, originAddr) && !addrCouldPrefix(t.LocalAddr, originAddr) {
			// nested targets extend this address, so none can match
			return
		}
		if t.Matches(mo) {
			if l := t.matchedAddrLen(mo); l > n {
				n = l
			}
		}
		for _, nt := range t.NestedTargets {
			walk(nt)
		}
	}
	walk(target)
	if n >= len(originAddr) {
		return true
	}
	return !allTargets().hasMoreSpecificMatch(mo, n)
}
