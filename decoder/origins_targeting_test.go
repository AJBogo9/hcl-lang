// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

// indexedPathReader serves reference contexts with an origin index, as
// tofu-ls does, and fails the test when a full path context is built.
type indexedPathReader struct {
	t     *testing.T
	paths map[string]*PathContext
}

func (r *indexedPathReader) Paths(ctx context.Context) []lang.Path {
	paths := make([]lang.Path, 0, len(r.paths))
	for path := range r.paths {
		paths = append(paths, lang.Path{Path: path})
	}
	return paths
}

func (r *indexedPathReader) PathContext(path lang.Path) (*PathContext, error) {
	r.t.Fatalf("a reference lookup built the full context of %q", path.Path)
	return nil, nil
}

func (r *indexedPathReader) ReferencePathContext(path lang.Path) (*PathContext, error) {
	pathCtx, ok := r.paths[path.Path]
	if !ok {
		return nil, fmt.Errorf("path not found: %q", path.Path)
	}
	indexed := *pathCtx
	indexed.ReferenceOriginIndex = reference.NewOriginIndex(pathCtx.ReferenceOrigins)
	return &indexed, nil
}

func TestOriginsTargeting_referenceContexts(t *testing.T) {
	root := t.TempDir()
	child := root + "/modules/child"
	rootPath := lang.Path{Path: root}
	childPath := lang.Path{Path: child}

	rng := func(file string, line int) hcl.Range {
		return hcl.Range{
			Filename: file,
			Start:    hcl.Pos{Line: line, Column: 1, Byte: line * 100},
			End:      hcl.Pos{Line: line, Column: 20, Byte: line*100 + 19},
		}
	}
	addr := func(steps ...string) lang.Address {
		a := lang.Address{lang.RootStep{Name: steps[0]}}
		for _, s := range steps[1:] {
			a = append(a, lang.AttrStep{Name: s})
		}
		return a
	}
	dynamic := reference.OriginConstraints{{OfType: cty.DynamicPseudoType}}

	varSize := reference.Target{Addr: addr("var", "size"), ScopeId: "variable", Type: cty.Number,
		RangePtr: rng("variables.tf", 1).Ptr(), DefRangePtr: rng("variables.tf", 1).Ptr()}
	outURL := reference.Target{Addr: addr("output", "url"), ScopeId: "output", Type: cty.DynamicPseudoType,
		RangePtr: rng("outputs.tf", 1).Ptr()}
	localName := reference.Target{Addr: addr("local", "name"), ScopeId: "local", Type: cty.DynamicPseudoType,
		RangePtr: rng("locals.tf", 1).Ptr()}

	paths := map[string]*PathContext{
		child: {
			ReferenceTargets: reference.Targets{varSize, outURL, localName},
			ReferenceOrigins: reference.Origins{
				reference.LocalOrigin{Addr: addr("var", "size"), Range: rng("main.tf", 1), Constraints: dynamic},
				reference.LocalOrigin{Addr: addr("local", "name"), Range: rng("main.tf", 2), Constraints: dynamic},
				reference.LocalOrigin{Addr: addr("var", "size"), Range: rng("outputs.tf", 2), Constraints: dynamic},
			},
		},
		root: {
			ReferenceTargets: reference.Targets{
				{Addr: addr("local", "name"), ScopeId: "local", Type: cty.DynamicPseudoType, RangePtr: rng("main.tf", 9).Ptr()},
			},
			ReferenceOrigins: reference.Origins{
				reference.PathOrigin{Range: rng("main.tf", 3), TargetAddr: addr("var", "size"), TargetPath: childPath,
					Constraints: reference.OriginConstraints{{OfScopeId: "variable", OfType: cty.DynamicPseudoType}}},
				reference.PathOrigin{Range: rng("main.tf", 4), TargetAddr: addr("output", "url"), TargetPath: childPath,
					Constraints: reference.OriginConstraints{{OfScopeId: "output", OfType: cty.DynamicPseudoType}}},
				// the root's own local.name is not the child's
				reference.LocalOrigin{Addr: addr("local", "name"), Range: rng("main.tf", 5), Constraints: dynamic},
			},
		},
	}

	sorted := func(origins []PathOrigin) []PathOrigin {
		sort.Slice(origins, func(i, j int) bool {
			if origins[i].Path.Path != origins[j].Path.Path {
				return origins[i].Path.Path < origins[j].Path.Path
			}
			return origins[i].Origin.OriginRange().Start.Byte < origins[j].Origin.OriginRange().Start.Byte
		})
		return origins
	}

	testCases := []struct {
		name     string
		target   reference.Target
		inPath   lang.Path
		expected []PathOrigin
	}{
		{
			"variable used in its module and set by the caller",
			varSize, childPath,
			[]PathOrigin{
				{Path: childPath, Origin: paths[child].ReferenceOrigins[0]},
				{Path: childPath, Origin: paths[child].ReferenceOrigins[2]},
			},
		},
		{
			"variable seen from the caller",
			varSize, rootPath,
			[]PathOrigin{
				{Path: rootPath, Origin: paths[root].ReferenceOrigins[0]},
			},
		},
		{
			"output used by the caller only",
			outURL, rootPath,
			[]PathOrigin{
				{Path: rootPath, Origin: paths[root].ReferenceOrigins[1]},
			},
		},
		{
			"local of the child",
			localName, childPath,
			[]PathOrigin{
				{Path: childPath, Origin: paths[child].ReferenceOrigins[1]},
			},
		},
	}

	plain := NewDecoder(&testPathReader{paths: paths})
	indexed := NewDecoder(&indexedPathReader{t: t, paths: paths})
	ctx := context.Background()
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			// the index finds what testing every origin finds
			all := sorted(plain.OriginsTargeting(ctx, tc.target, childPath))
			if diff := cmp.Diff(all, sorted(indexed.OriginsTargeting(ctx, tc.target, childPath)), ctydebug.CmpOptions); diff != "" {
				t.Fatalf("indexed lookup differs: %s", diff)
			}

			// one path is the part of all paths in that path
			inPath := make([]PathOrigin, 0)
			for _, o := range all {
				if o.Path.Equals(tc.inPath) {
					inPath = append(inPath, o)
				}
			}
			if diff := cmp.Diff(inPath, sorted(indexed.OriginsTargetingInPath(tc.target, childPath, tc.inPath)), ctydebug.CmpOptions); diff != "" {
				t.Fatalf("lookup in %s differs from all paths: %s", tc.inPath.Path, diff)
			}
			if diff := cmp.Diff(tc.expected, inPath, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("unexpected origins: %s", diff)
			}
		})
	}
}
