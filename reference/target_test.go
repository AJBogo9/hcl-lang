// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestTarget_Address(t *testing.T) {
	testCases := []struct {
		name            string
		pos             hcl.Pos
		activeSelfRefs  bool
		target          Target
		expectedAddress lang.Address
	}{
		{
			"absolute address and no local address",
			hcl.InitialPos,
			false,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "aws_instance"},
					lang.AttrStep{Name: "instance_size"},
				},
			},
			lang.Address{
				lang.RootStep{Name: "aws_instance"},
				lang.AttrStep{Name: "instance_size"},
			},
		},
		{
			"local address and no absolute address",
			hcl.InitialPos,
			false,
			Target{
				LocalAddr: lang.Address{
					lang.RootStep{Name: "count"},
					lang.AttrStep{Name: "index"},
				},
			},
			lang.Address{
				lang.RootStep{Name: "count"},
				lang.AttrStep{Name: "index"},
			},
		},
		{
			"self address with active self and matching range",
			hcl.Pos{Line: 2, Column: 2, Byte: 2},
			true,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "aws_instance"},
					lang.AttrStep{Name: "instance_size"},
				},
				LocalAddr: lang.Address{
					lang.RootStep{Name: "self"},
					lang.AttrStep{Name: "instance_size"},
				},
				TargetableFromRangePtr: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 3, Column: 1, Byte: 10},
				},
			},
			lang.Address{
				lang.RootStep{Name: "self"},
				lang.AttrStep{Name: "instance_size"},
			},
		},
		{
			"self address without active self but matching range",
			hcl.Pos{Line: 2, Column: 2, Byte: 2},
			false,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "aws_instance"},
					lang.AttrStep{Name: "instance_size"},
				},
				LocalAddr: lang.Address{
					lang.RootStep{Name: "self"},
					lang.AttrStep{Name: "instance_size"},
				},
				TargetableFromRangePtr: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 3, Column: 1, Byte: 10},
				},
			},
			lang.Address{
				lang.RootStep{Name: "aws_instance"},
				lang.AttrStep{Name: "instance_size"},
			},
		},
		{
			"self address with active self but no matching range",
			hcl.Pos{Line: 5, Column: 2, Byte: 15},
			true,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "aws_instance"},
					lang.AttrStep{Name: "instance_size"},
				},
				LocalAddr: lang.Address{
					lang.RootStep{Name: "self"},
					lang.AttrStep{Name: "instance_size"},
				},
				TargetableFromRangePtr: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 3, Column: 1, Byte: 10},
				},
			},
			lang.Address{
				lang.RootStep{Name: "aws_instance"},
				lang.AttrStep{Name: "instance_size"},
			},
		},
		{
			"self address with active self and missing targetable",
			hcl.Pos{Line: 5, Column: 2, Byte: 15},
			true,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "aws_instance"},
					lang.AttrStep{Name: "instance_size"},
				},
				LocalAddr: lang.Address{
					lang.RootStep{Name: "self"},
					lang.AttrStep{Name: "instance_size"},
				},
			},
			lang.Address{
				lang.RootStep{Name: "aws_instance"},
				lang.AttrStep{Name: "instance_size"},
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			ctx := context.Background()

			if tc.activeSelfRefs {
				ctx = schema.WithActiveSelfRefs(ctx)
			}

			address := tc.target.Address(ctx, tc.pos)
			if diff := cmp.Diff(tc.expectedAddress, address, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatch of address: %s", diff)
			}
		})
	}
}

func TestTarget_Matches_elementOf(t *testing.T) {
	root := func(name string) lang.RootStep { return lang.RootStep{Name: name} }
	attr := func(name string) lang.AttrStep { return lang.AttrStep{Name: name} }
	idx := func(key cty.Value) lang.IndexStep { return lang.IndexStep{Key: key} }
	names := lang.Address{root("var"), attr("names")}

	testCases := []struct {
		name       string
		targetType cty.Type
		addr       lang.Address
		cons       OriginConstraints
		matches    bool
	}{
		{"element of a list", cty.List(cty.String), lang.Address{root("var"), attr("names"), idx(cty.NumberIntVal(0))}, OriginConstraints{{OfType: cty.String}}, true},
		{"attribute of an element", cty.List(cty.Object(map[string]cty.Type{"id": cty.String})), lang.Address{root("var"), attr("names"), idx(cty.NumberIntVal(0)), attr("id")}, OriginConstraints{{OfType: cty.DynamicPseudoType}}, true},
		{"value of a map", cty.Map(cty.Number), lang.Address{root("var"), attr("names"), idx(cty.StringVal("a"))}, OriginConstraints{{OfType: cty.Number}}, true},
		{"key into an object", cty.Object(map[string]cty.Type{"a": cty.String}), lang.Address{root("var"), attr("names"), idx(cty.StringVal("a"))}, nil, true},
		{"other scope", cty.List(cty.String), lang.Address{root("var"), attr("names"), idx(cty.NumberIntVal(0))}, OriginConstraints{{OfScopeId: lang.ScopeId("local")}}, false},
		{"a string has no elements", cty.String, lang.Address{root("var"), attr("names"), idx(cty.NumberIntVal(0))}, nil, false},
		{"attribute, not an index", cty.List(cty.String), lang.Address{root("var"), attr("names"), attr("x")}, nil, false},
		{"other variable", cty.List(cty.String), lang.Address{root("var"), attr("other"), idx(cty.NumberIntVal(0))}, nil, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := Target{Addr: names, ScopeId: lang.ScopeId("variable"), Type: tc.targetType}
			origin := LocalOrigin{Addr: tc.addr, Constraints: tc.cons}
			if got := target.Matches(origin); got != tc.matches {
				t.Fatalf("expected match=%t, got %t", tc.matches, got)
			}
		})
	}
}

func TestTarget_Matches_scopedOnly(t *testing.T) {
	addr := lang.Address{lang.RootStep{Name: "local"}, lang.AttrStep{Name: "secondary"}}
	provider := Target{Addr: addr, ScopeId: lang.ScopeId("provider"), Type: cty.DynamicPseudoType, ScopedOnly: true}
	testCases := []struct {
		name    string
		cons    OriginConstraints
		matches bool
	}{
		{"provider meta-argument", OriginConstraints{{OfScopeId: lang.ScopeId("provider"), OfType: cty.DynamicPseudoType}}, true},
		{"any value elsewhere", OriginConstraints{{OfType: cty.String}}, false},
		{"unconstrained", nil, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := provider.Matches(LocalOrigin{Addr: addr, Constraints: tc.cons}); got != tc.matches {
				t.Fatalf("expected match=%t, got %t", tc.matches, got)
			}
		})
	}
}

func TestTarget_Matches_targetableFromRanges(t *testing.T) {
	addr := lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "names"}}
	rng := func(file string, start, end int) hcl.Range {
		return hcl.Range{
			Filename: file,
			Start:    hcl.Pos{Line: 1, Column: start + 1, Byte: start},
			End:      hcl.Pos{Line: 1, Column: end + 1, Byte: end},
		}
	}
	runs := []hcl.Range{rng("a.tftest.hcl", 10, 20), rng("a.tftest.hcl", 40, 50)}

	testCases := []struct {
		name    string
		ranges  []hcl.Range
		origin  MatchableOrigin
		matches bool
	}{
		{"no ranges", nil, LocalOrigin{Addr: addr, Range: rng("a.tftest.hcl", 0, 5)}, true},
		{"inside the first range", runs, LocalOrigin{Addr: addr, Range: rng("a.tftest.hcl", 12, 15)}, true},
		{"inside the second range", runs, LocalOrigin{Addr: addr, Range: rng("a.tftest.hcl", 42, 45)}, true},
		{"between the ranges", runs, LocalOrigin{Addr: addr, Range: rng("a.tftest.hcl", 25, 30)}, false},
		{"same offsets in another file", runs, LocalOrigin{Addr: addr, Range: rng("b.tftest.hcl", 12, 15)}, false},
		{"path origin inside a range", runs, PathOrigin{TargetAddr: addr, Range: rng("a.tftest.hcl", 12, 15)}, true},
		{"path origin outside the ranges", runs, PathOrigin{TargetAddr: addr, Range: rng("a.tftest.hcl", 25, 30)}, false},
		{"element outside the ranges", runs, LocalOrigin{
			Addr:  append(addr.Copy(), lang.IndexStep{Key: cty.NumberIntVal(0)}),
			Range: rng("a.tftest.hcl", 25, 30),
		}, false},
		{"element inside a range", runs, LocalOrigin{
			Addr:  append(addr.Copy(), lang.IndexStep{Key: cty.NumberIntVal(0)}),
			Range: rng("a.tftest.hcl", 12, 15),
		}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := Target{
				Addr:                 addr,
				ScopeId:              lang.ScopeId("variable"),
				Type:                 cty.List(cty.String),
				TargetableFromRanges: tc.ranges,
			}
			if got := target.Matches(tc.origin); got != tc.matches {
				t.Fatalf("expected match=%t, got %t", tc.matches, got)
			}
		})
	}
}

func TestTargets_MatchWalk_targetableFromRanges(t *testing.T) {
	rng := func(start, end int) hcl.Range {
		return hcl.Range{
			Filename: "a.tftest.hcl",
			Start:    hcl.Pos{Line: 1, Column: start + 1, Byte: start},
			End:      hcl.Pos{Line: 1, Column: end + 1, Byte: end},
		}
	}
	targets := Targets{
		{
			Addr:                 lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "root"}},
			Type:                 cty.String,
			TargetableFromRanges: []hcl.Range{rng(0, 20)},
		},
		{
			Addr:                 lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "child"}},
			Type:                 cty.String,
			TargetableFromRanges: []hcl.Range{rng(30, 50)},
		},
		{
			LocalAddr:            lang.Address{lang.RootStep{Name: "run"}, lang.AttrStep{Name: "first"}},
			Type:                 cty.DynamicPseudoType,
			TargetableFromRanges: []hcl.Range{rng(30, 50)},
		},
	}

	testCases := []struct {
		name   string
		origin hcl.Range
		want   []string
	}{
		{"in the first range", rng(5, 5), []string{"var.root"}},
		{"in the second range", rng(35, 35), []string{"var.child", "run.first"}},
		{"outside both", rng(25, 25), []string{}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := make([]string, 0)
			targets.MatchWalk(context.Background(), schema.Reference{OfType: cty.String}, "", hcl.Range{}, tc.origin, func(target Target) error {
				if len(target.Addr) > 0 {
					got = append(got, target.Addr.String())
				} else {
					got = append(got, target.LocalAddr.String())
				}
				return nil
			})
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("unexpected matches: %s", diff)
			}
		})
	}
}
