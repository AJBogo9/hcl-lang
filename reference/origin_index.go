// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"github.com/hashicorp/hcl-lang/lang"
)

// OriginIndex files origins under the first one and the first two steps
// of their address. A target (or a nested target) only matches origins
// whose address starts with the target's address, so its candidates are
// the origins filed under the first steps of that address.
//
// Matching through an index gives the results of Origins.Match and
// Origins.MatchResolved, in the same order, without testing every origin
// against the target and each of its nested targets.
//
// An OriginIndex is read-only once built and safe for concurrent use.
type OriginIndex struct {
	origins Origins
	// positions in origins, in ascending order
	byKey map[indexKey][]int
}

// indexKey is the first step, or the first two steps, of an address.
type indexKey struct {
	first, second string
	steps         int
}

func keyOfAddress(addr lang.Address) (indexKey, bool) {
	switch len(addr) {
	case 0:
		// an empty address equals no address, so it matches nothing
		return indexKey{}, false
	case 1:
		return indexKey{first: addr[0].String(), steps: 1}, true
	}
	return indexKey{first: addr[0].String(), second: addr[1].String(), steps: 2}, true
}

// NewOriginIndex indexes the origins, which must not change afterwards.
func NewOriginIndex(origins Origins) *OriginIndex {
	idx := &OriginIndex{
		origins: origins,
		byKey:   make(map[indexKey][]int),
	}
	for i, origin := range origins {
		var addr lang.Address
		switch o := origin.(type) {
		case LocalOrigin:
			addr = o.Address()
		case PathOrigin:
			addr = o.Address()
		default:
			// Match only ever returns local and path origins
			continue
		}
		if len(addr) == 0 {
			continue
		}
		one := indexKey{first: addr[0].String(), steps: 1}
		idx.byKey[one] = append(idx.byKey[one], i)
		if len(addr) > 1 {
			two := indexKey{first: one.first, second: addr[1].String(), steps: 2}
			idx.byKey[two] = append(idx.byKey[two], i)
		}
	}
	return idx
}

// Origins returns the indexed origins.
func (idx *OriginIndex) Origins() Origins {
	return idx.origins
}

// TargetMatcher is a target prepared for matching against origin
// indexes: the index keys of the target and of its nested targets are
// worked out once, however many indexes it is matched against.
type TargetMatcher struct {
	target Target
	// the target and its nested targets, depth first, in the order
	// Origins.Match visits them
	nodes []matcherNode
	// the distinct keys of all nodes
	keys []indexKey
}

type matcherNode struct {
	target Target
	// positions in TargetMatcher.keys of the keys of Addr and LocalAddr
	keys []int
}

// NewTargetMatcher prepares the target for matching against indexes.
func NewTargetMatcher(target Target) *TargetMatcher {
	m := &TargetMatcher{target: target}
	positions := make(map[indexKey]int)
	var walk func(t Target)
	walk = func(t Target) {
		node := matcherNode{target: t}
		for _, addr := range []lang.Address{t.Addr, t.LocalAddr} {
			key, ok := keyOfAddress(addr)
			if !ok {
				continue
			}
			pos, ok := positions[key]
			if !ok {
				pos = len(m.keys)
				positions[key] = pos
				m.keys = append(m.keys, key)
			}
			if len(node.keys) == 0 || node.keys[0] != pos {
				node.keys = append(node.keys, pos)
			}
		}
		m.nodes = append(m.nodes, node)
		for _, nested := range t.NestedTargets {
			walk(nested)
		}
	}
	walk(target)
	return m
}

// Target returns the target the matcher was prepared for.
func (m *TargetMatcher) Target() Target {
	return m.target
}

// Match is Origins.Match for the indexed origins.
func (idx *OriginIndex) Match(localPath lang.Path, m *TargetMatcher, targetPath lang.Path) Origins {
	origins := make(Origins, 0)

	candidates := make([][]int, len(m.keys))
	found := false
	for i, key := range m.keys {
		candidates[i] = idx.byKey[key]
		if len(candidates[i]) > 0 {
			found = true
		}
	}
	if !found {
		return origins
	}

	samePath := localPath.Equals(targetPath)
	visit := func(t Target, i int) {
		switch origin := idx.origins[i].(type) {
		case LocalOrigin:
			if samePath && t.Matches(origin) {
				origins = append(origins, origin)
			}
		case PathOrigin:
			if origin.TargetPath.Equals(targetPath) && t.Matches(origin) {
				origins = append(origins, origin)
			}
		}
	}

	for _, node := range m.nodes {
		switch len(node.keys) {
		case 0:
			continue
		case 1:
			for _, i := range candidates[node.keys[0]] {
				visit(node.target, i)
			}
		default:
			// both lists are ascending: merge them, visiting each
			// origin once and in the order of the origins
			a, b := candidates[node.keys[0]], candidates[node.keys[1]]
			for len(a) > 0 || len(b) > 0 {
				switch {
				case len(b) == 0 || (len(a) > 0 && a[0] < b[0]):
					visit(node.target, a[0])
					a = a[1:]
				case len(a) == 0 || b[0] < a[0]:
					visit(node.target, b[0])
					b = b[1:]
				default:
					visit(node.target, a[0])
					a, b = a[1:], b[1:]
				}
			}
		}
	}

	return origins
}

// MatchResolved is Origins.MatchResolved for the indexed origins.
func (idx *OriginIndex) MatchResolved(localPath lang.Path, m *TargetMatcher, targetPath lang.Path, allTargets func() Targets) Origins {
	origins := make(Origins, 0)

	for _, refOrigin := range idx.Match(localPath, m, targetPath) {
		if !originResolvesTo(refOrigin, m.target, allTargets) {
			continue
		}
		origins = append(origins, refOrigin)
	}

	return origins
}
