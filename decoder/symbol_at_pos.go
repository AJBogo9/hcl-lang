// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"

	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// PathTarget is a reference target together with the path
// (module and language) which declares it.
type PathTarget struct {
	Path   lang.Path
	Target reference.Target
}

// PathOrigin is a reference origin together with the path
// (module and language) it is found in.
type PathOrigin struct {
	Path   lang.Path
	Origin reference.Origin
}

// SymbolTargetsAtPos returns the targets of the symbol under the cursor.
//
// When pos is on a reference (origin), these are the targets that
// reference resolves to, possibly declared in another path. Otherwise
// they are the innermost targets declared at pos, e.g. the variable
// whose block header pos is on. The second return value tells whether
// pos was on a reference.
func (d *Decoder) SymbolTargetsAtPos(path lang.Path, file string, pos hcl.Pos) ([]PathTarget, bool) {
	localCtx, err := ReferencePathContext(d.pathReader, path)
	if err != nil {
		return nil, false
	}

	if origins, ok := localCtx.ReferenceOrigins.AtPos(file, pos); ok {
		targets := make([]PathTarget, 0)
		for _, origin := range origins {
			if lo, ok := origin.(reference.LocalOrigin); ok {
				origin = narrowToStepAtPos(localCtx, lo, file, pos)
			}
			targets = append(targets, d.resolveOrigin(path, localCtx, origin)...)
		}
		if len(targets) > 0 {
			return targets, true
		}
	}

	innermost, ok := localCtx.ReferenceTargets.InnermostAtPos(file, pos)
	if !ok {
		return nil, false
	}
	targets := make([]PathTarget, 0, len(innermost))
	for _, target := range innermost {
		targets = append(targets, PathTarget{Path: path, Target: target})
	}
	return targets, false
}

// narrowToStepAtPos shortens the origin's address to the step under the
// cursor, so that on "web" in aws_instance.web.id the symbol is the
// resource and not its id attribute. It keeps at least two steps, since
// a root such as var or local alone is not a symbol (and "local" alone
// would be a provider named local). The origin is returned unchanged
// when the shorter address resolves to nothing.
func narrowToStepAtPos(pathCtx *PathContext, origin reference.LocalOrigin, file string, pos hcl.Pos) reference.Origin {
	f, ok := pathCtx.Files[file]
	if !ok || f == nil {
		return origin
	}
	rng := origin.Range
	if rng.End.Byte > len(f.Bytes) || rng.Start.Byte >= rng.End.Byte {
		return origin
	}
	traversal, _ := hclsyntax.ParseTraversalPartial(f.Bytes[rng.Start.Byte:rng.End.Byte], file, rng.Start)

	stepIdx := -1
	for i, step := range traversal {
		stepRng := step.SourceRange()
		if stepRng.ContainsPos(pos) || stepRng.End == pos {
			stepIdx = i
			break
		}
	}
	if stepIdx < 0 {
		return origin
	}

	for n := stepIdx + 1; n < len(origin.Addr); n++ {
		if n < 2 {
			continue
		}
		narrowed := reference.LocalOrigin{
			Addr:  origin.Addr.FirstSteps(uint(n)),
			Range: origin.Range,
			Constraints: reference.OriginConstraints{
				{OfType: cty.DynamicPseudoType},
			},
		}
		if targets, ok := pathCtx.ReferenceTargets.Match(narrowed); ok {
			for _, t := range targets {
				if len(t.Addr) == n || len(t.LocalAddr) == n {
					return narrowed
				}
			}
		}
	}
	return origin
}

func (d *Decoder) resolveOrigin(path lang.Path, pathCtx *PathContext, origin reference.Origin) []PathTarget {
	targetPath := path
	targetCtx := pathCtx

	switch o := origin.(type) {
	case reference.DirectOrigin:
		// points to a file, not to a symbol
		return nil
	case reference.PathOrigin:
		ctx, err := ReferencePathContext(d.pathReader, o.TargetPath)
		if err != nil {
			return nil
		}
		targetCtx = ctx
		targetPath = o.TargetPath
	}

	matchableOrigin, ok := origin.(reference.MatchableOrigin)
	if !ok {
		return nil
	}
	matched, ok := targetCtx.ReferenceTargets.Match(matchableOrigin)
	if !ok {
		return nil
	}
	targets := make([]PathTarget, 0, len(matched))
	for _, target := range matched {
		if target.RangePtr == nil {
			// not addressable
			continue
		}
		targets = append(targets, PathTarget{Path: targetPath, Target: target})
	}
	return targets
}

// ReferencePathReader is implemented by a PathReader which can return
// the part of a path context that reference lookups read: the reference
// origins and targets and the files. Building it skips the schema, which
// is costly, and it may be cached and shared, so it is read-only.
// Lookups across paths, such as OriginsTargeting, use it when the path
// reader implements it.
type ReferencePathReader interface {
	ReferencePathContext(path lang.Path) (*PathContext, error)
}

// ReferencePathContext returns the context of the path for reference
// lookups: from ReferencePathContext when the reader implements
// ReferencePathReader, else from PathContext.
func ReferencePathContext(pathReader PathReader, path lang.Path) (*PathContext, error) {
	if rr, ok := pathReader.(ReferencePathReader); ok {
		return rr.ReferencePathContext(path)
	}
	return pathReader.PathContext(path)
}

// OriginsTargeting returns origins from all known paths which resolve
// to the given target declared in targetPath (or to one of its nested
// targets), in no particular order.
func (d *Decoder) OriginsTargeting(ctx context.Context, target reference.Target, targetPath lang.Path) []PathOrigin {
	return OriginsTargeting(ctx, d.pathReader, target, targetPath)
}

// OriginsTargeting is like Decoder.OriginsTargeting for callers
// which only have a PathReader, such as code lenses.
func OriginsTargeting(ctx context.Context, pathReader PathReader, target reference.Target, targetPath lang.Path) []PathOrigin {
	return originsTargeting(pathReader, target, targetPath, pathReader.Paths(ctx))
}

// OriginsTargetingInPath is like OriginsTargeting, but only returns the
// origins found in inPath, e.g. to highlight the uses in one file. It
// reads no other path than inPath and, when an origin needs resolving,
// targetPath.
func (d *Decoder) OriginsTargetingInPath(target reference.Target, targetPath lang.Path, inPath lang.Path) []PathOrigin {
	return originsTargeting(d.pathReader, target, targetPath, []lang.Path{inPath})
}

func originsTargeting(pathReader PathReader, target reference.Target, targetPath lang.Path, paths []lang.Path) []PathOrigin {
	origins := make([]PathOrigin, 0)

	// read only when an origin needs resolving, since building
	// a path context is not free
	var targetCtx *PathContext
	allTargets := func() reference.Targets {
		if targetCtx == nil {
			ctx, err := ReferencePathContext(pathReader, targetPath)
			if err != nil {
				return reference.Targets{}
			}
			targetCtx = ctx
		}
		return targetCtx.ReferenceTargets
	}

	// prepared once for all the indexed paths
	var matcher *reference.TargetMatcher

	for _, p := range paths {
		pathCtx, err := ReferencePathContext(pathReader, p)
		if err != nil {
			continue
		}
		if p.Equals(targetPath) {
			targetCtx = pathCtx
		}
		var matched reference.Origins
		if pathCtx.ReferenceOriginIndex != nil {
			if matcher == nil {
				matcher = reference.NewTargetMatcher(target)
			}
			matched = pathCtx.ReferenceOriginIndex.MatchResolved(p, matcher, targetPath, allTargets)
		} else {
			matched = pathCtx.ReferenceOrigins.MatchResolved(p, target, targetPath, allTargets)
		}
		for _, origin := range matched {
			origins = append(origins, PathOrigin{Path: p, Origin: origin})
		}
	}

	return origins
}
