// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestOriginIndex_matchesOrigins(t *testing.T) {
	root := lang.Path{Path: "/ws", LanguageID: "opentofu"}
	child := lang.Path{Path: "/ws/modules/child", LanguageID: "opentofu"}
	vars := lang.Path{Path: "/ws", LanguageID: "opentofu-vars"}

	rng := func(line int) *hcl.Range {
		return &hcl.Range{
			Filename: "main.tf",
			Start:    hcl.Pos{Line: line, Column: 1, Byte: line * 100},
			End:      hcl.Pos{Line: line + 2, Column: 2, Byte: line*100 + 50},
		}
	}
	instance := Target{
		Addr:     lang.Address{lang.RootStep{Name: "aws_instance"}, lang.AttrStep{Name: "web"}},
		ScopeId:  lang.ScopeId("resource"),
		Type:     cty.Object(map[string]cty.Type{"id": cty.String, "tags": cty.Map(cty.String)}),
		RangePtr: rng(10),
		NestedTargets: Targets{
			{
				Addr: lang.Address{lang.RootStep{Name: "aws_instance"}, lang.AttrStep{Name: "web"}, lang.AttrStep{Name: "id"}},
				Type: cty.String,
			},
			{
				Addr: lang.Address{lang.RootStep{Name: "aws_instance"}, lang.AttrStep{Name: "web"}, lang.AttrStep{Name: "tags"}},
				Type: cty.Map(cty.String),
			},
		},
	}
	instanceTypeless := Target{
		Addr:     instance.Addr,
		ScopeId:  lang.ScopeId("resource"),
		RangePtr: rng(10),
	}
	self := Target{
		LocalAddr:              lang.Address{lang.RootStep{Name: "self"}},
		Addr:                   instance.Addr,
		Type:                   cty.DynamicPseudoType,
		TargetableFromRangePtr: rng(10),
		RangePtr:               rng(10),
	}
	moduleCall := Target{
		Addr:     lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}},
		ScopeId:  lang.ScopeId("module"),
		Type:     cty.DynamicPseudoType,
		RangePtr: rng(20),
	}
	childVar := Target{
		Addr:     lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "size"}},
		ScopeId:  lang.ScopeId("variable"),
		Type:     cty.Number,
		RangePtr: rng(30),
	}
	childOutput := Target{
		Addr:     lang.Address{lang.RootStep{Name: "output"}, lang.AttrStep{Name: "url"}},
		ScopeId:  lang.ScopeId("output"),
		Type:     cty.DynamicPseudoType,
		RangePtr: rng(40),
	}
	empty := Target{Type: cty.DynamicPseudoType, RangePtr: rng(50)}

	anyCons := func(scope lang.ScopeId) OriginConstraints {
		return OriginConstraints{{OfScopeId: scope, OfType: cty.DynamicPseudoType}}
	}
	moduleInstance := localOrigin(9, "", "module", "app")
	moduleInstance.Addr = lang.Address{
		lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"},
		lang.IndexStep{Key: cty.NumberIntVal(0)}, lang.AttrStep{Name: "url"},
	}
	origins := Origins{
		localOrigin(1, "", "local", "name_prefix"),
		localOrigin(2, "provider", "local", "secondary"),
		localOrigin(3, "provider", "local"),
		localOrigin(4, "", "var", "network", "zones"),
		localOrigin(5, "", "local", "undeclared"),
		localOrigin(6, "resource", "aws_instance", "web"),
		localOrigin(7, "", "aws_instance", "web", "id"),
		localOrigin(8, "", "aws_instance", "web", "tags", "Name"),
		moduleInstance,
		localOrigin(11, "", "self", "id"),
		localOrigin(12, "", "module", "app", "url"),
		PathOrigin{
			Range:       hcl.Range{Filename: "main.tf", Start: hcl.Pos{Line: 13, Byte: 1300}, End: hcl.Pos{Line: 13, Byte: 1310}},
			TargetAddr:  lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "size"}},
			TargetPath:  child,
			Constraints: anyCons("variable"),
		},
		PathOrigin{
			Range:       hcl.Range{Filename: "main.tf", Start: hcl.Pos{Line: 14, Byte: 1400}, End: hcl.Pos{Line: 14, Byte: 1420}},
			TargetAddr:  lang.Address{lang.RootStep{Name: "output"}, lang.AttrStep{Name: "url"}},
			TargetPath:  child,
			Constraints: anyCons("output"),
		},
		DirectOrigin{
			Range:       hcl.Range{Filename: "main.tf", Start: hcl.Pos{Line: 15, Byte: 1500}, End: hcl.Pos{Line: 15, Byte: 1510}},
			TargetPath:  child,
			TargetRange: *rng(1),
		},
		localOrigin(16, "", "var", "network"),
		localOrigin(17, "", "var", "network"),
	}
	allTargets := Targets{providerLocal, providerLocalSecondary, localNamePrefix, varNetwork, instance, moduleCall}

	targets := []Target{providerLocal, providerLocalSecondary, localNamePrefix, varNetwork,
		instance, instanceTypeless, self, moduleCall, childVar, childOutput, empty}
	paths := []lang.Path{root, child, vars}

	idx := NewOriginIndex(origins)
	count := 0
	for _, target := range targets {
		m := NewTargetMatcher(target)
		for _, localPath := range paths {
			for _, targetPath := range paths {
				name := fmt.Sprintf("%s-%s-%s", target.Addr.String()+target.LocalAddr.String(), localPath.LanguageID+localPath.Path, targetPath.Path)
				t.Run(name, func(t *testing.T) {
					expected := origins.Match(localPath, target, targetPath)
					got := idx.Match(localPath, m, targetPath)
					if diff := cmp.Diff(expected, got, ctydebug.CmpOptions); diff != "" {
						t.Fatalf("Match differs: %s", diff)
					}
					count += len(got)

					all := func() Targets { return allTargets }
					expected = origins.MatchResolved(localPath, target, targetPath, all)
					got = idx.MatchResolved(localPath, m, targetPath, all)
					if diff := cmp.Diff(expected, got, ctydebug.CmpOptions); diff != "" {
						t.Fatalf("MatchResolved differs: %s", diff)
					}
				})
			}
		}
	}
	// the cases must exercise matching, not only agree on nothing
	if count < 15 {
		t.Fatalf("expected the cases to match at least 15 origins, got %d", count)
	}
}

func BenchmarkOriginIndex_Match(b *testing.B) {
	path := lang.Path{Path: "/ws", LanguageID: "opentofu"}
	origins := make(Origins, 0)
	for i := 0; i < 12000; i++ {
		origins = append(origins, localOrigin(i, "", "terraform_data", fmt.Sprintf("r%d", i/3), "output"))
	}
	target := Target{
		Addr:     lang.Address{lang.RootStep{Name: "terraform_data"}, lang.AttrStep{Name: "r2000"}},
		Type:     cty.DynamicPseudoType,
		RangePtr: &hcl.Range{Filename: "main.tf"},
	}
	idx := NewOriginIndex(origins)
	m := NewTargetMatcher(target)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(idx.Match(path, m, path)) != 3 {
			b.Fatal("expected 3 origins")
		}
	}
}
