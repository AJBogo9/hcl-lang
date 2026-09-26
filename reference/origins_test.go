// Copyright (c) HashiCorp, Inc.
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

func TestOrigins_AtPos(t *testing.T) {
	testCases := []struct {
		name            string
		origins         Origins
		pos             hcl.Pos
		expectedOrigins Origins
		expectedFound   bool
	}{
		{
			"no origins",
			Origins{},
			hcl.InitialPos,
			Origins{},
			false,
		},
		{
			"single mismatching origin",
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "blah"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
					},
				},
			},
			hcl.Pos{
				Line:   1,
				Column: 3,
				Byte:   2,
			},
			Origins{},
			false,
		},
		{
			"single matching origin",
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "blah"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
					},
				},
			},
			hcl.Pos{
				Line:   1,
				Column: 9,
				Byte:   8,
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "blah"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
					},
				},
			},
			true,
		},
		{
			"multiple origins - single match",
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "foo"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 1, Column: 8, Byte: 7},
						End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
					},
					Range: hcl.Range{
						Filename: "differentfile.tf",
						Start:    hcl.Pos{Line: 2, Column: 8, Byte: 14},
						End:      hcl.Pos{Line: 2, Column: 12, Byte: 18},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "bar"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 2, Column: 8, Byte: 14},
						End:      hcl.Pos{Line: 2, Column: 12, Byte: 18},
					},
				},
			},
			hcl.Pos{
				Line:   2,
				Column: 9,
				Byte:   15,
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "bar"},
					},
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 2, Column: 8, Byte: 14},
						End:      hcl.Pos{Line: 2, Column: 12, Byte: 18},
					},
				},
			},
			true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			origins, ok := tc.origins.AtPos("test.tf", tc.pos)
			if !ok && tc.expectedFound {
				t.Fatal("expected origin to be found")
			}

			if diff := cmp.Diff(tc.expectedOrigins, origins, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatched origin: %s", diff)
			}
		})
	}
}

func TestOrigins_Match(t *testing.T) {
	alphaPath := lang.Path{Path: t.TempDir()}
	betaPath := lang.Path{Path: t.TempDir()}

	testCases := []struct {
		name            string
		localPath       lang.Path
		origins         Origins
		targetPath      lang.Path
		target          Target
		expectedOrigins Origins
	}{
		{
			"no origins",
			alphaPath,
			Origins{},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
				},
				Type: cty.String,
			},
			Origins{},
		},
		{
			"exact address match",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "secondstep"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
					lang.AttrStep{Name: "secondstep"},
				},
				Type: cty.String,
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "secondstep"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
		},
		{
			"no match",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "secondstep"},
					},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
					lang.AttrStep{Name: "different"},
				},
				Type: cty.String,
			},
			Origins{},
		},
		{
			"match of nested target - two matches",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "foo"},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.DynamicPseudoType},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "second"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
				},
				Type: cty.Object(map[string]cty.Type{
					"second": cty.String,
				}),
				NestedTargets: Targets{
					{
						Addr: lang.Address{
							lang.RootStep{Name: "test"},
							lang.AttrStep{Name: "second"},
						},
						Type: cty.String,
					},
				},
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.DynamicPseudoType},
					},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "second"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
		},
		{
			"loose match of target of unknown type",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "foo"},
					},
					Constraints: OriginConstraints{{}},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
					Constraints: OriginConstraints{{}},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "second"},
					},
					Constraints: OriginConstraints{{}},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
				},
				Type: cty.DynamicPseudoType,
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
					Constraints: OriginConstraints{{}},
				},
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
						lang.AttrStep{Name: "second"},
					},
					Constraints: OriginConstraints{{}},
				},
			},
		},
		{
			"mismatch of target nil type",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "test"},
					},
					Constraints: OriginConstraints{
						{OfScopeId: lang.ScopeId("test")},
					},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
				},
				ScopeId: lang.ScopeId("test"),
				Type:    cty.String,
			},
			Origins{},
		},
		// JSON edge cases
		{
			"constraint-less origin mismatching scope-only target",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "alpha"},
					},
					Constraints: nil,
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "var"},
					lang.AttrStep{Name: "alpha"},
				},
				ScopeId: "variable",
				Type:    cty.NilType,
			},
			Origins{},
		},
		{
			"constraint-less origin matching type-aware target",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					Constraints: nil,
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "var"},
					lang.AttrStep{Name: "beta"},
				},
				ScopeId: "variable",
				Type:    cty.DynamicPseudoType,
			},
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					Constraints: nil,
				},
			},
		},
		{
			"cross-path mis-match",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
			betaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "var"},
					lang.AttrStep{Name: "beta"},
				},
				ScopeId: "variable",
				Type:    cty.String,
			},
			Origins{},
		},
		{
			"cross-path match",
			alphaPath,
			Origins{
				LocalOrigin{
					Addr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
				PathOrigin{
					TargetAddr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					TargetPath: betaPath,
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
			betaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "var"},
					lang.AttrStep{Name: "beta"},
				},
				ScopeId: "variable",
				Type:    cty.String,
			},
			Origins{
				PathOrigin{
					TargetAddr: lang.Address{
						lang.RootStep{Name: "var"},
						lang.AttrStep{Name: "beta"},
					},
					TargetPath: betaPath,
					Constraints: OriginConstraints{
						{OfType: cty.String},
					},
				},
			},
		},
		{
			"direct origin cannot be matched",
			alphaPath,
			Origins{
				DirectOrigin{
					Range: hcl.Range{
						Filename: "origin.tf",
						Start:    hcl.InitialPos,
						End:      hcl.InitialPos,
					},
					TargetPath: betaPath,
					TargetRange: hcl.Range{
						Filename: "target.tf",
						Start:    hcl.InitialPos,
						End:      hcl.InitialPos,
					},
				},
			},
			alphaPath,
			Target{
				Addr: lang.Address{
					lang.RootStep{Name: "test"},
				},
				Type: cty.String,
			},
			Origins{},
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			origins := tc.origins.Match(tc.localPath, tc.target, tc.targetPath)

			if diff := cmp.Diff(tc.expectedOrigins, origins, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatched reference origins: %s", diff)
			}
		})
	}
}

func TestLocalOrigin_Address_moduleInstanceKey(t *testing.T) {
	output := Target{
		Addr: lang.Address{
			lang.RootStep{Name: "module"},
			lang.AttrStep{Name: "app"},
			lang.AttrStep{Name: "url"},
		},
		ScopeId: lang.ScopeId("module"),
		Type:    cty.String,
	}
	testCases := []struct {
		name    string
		addr    lang.Address
		matches bool
	}{
		{"no key", lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.AttrStep{Name: "url"}}, true},
		{"count index", lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.NumberIntVal(0)}, lang.AttrStep{Name: "url"}}, true},
		{"for_each key", lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.StringVal("a")}, lang.AttrStep{Name: "url"}}, true},
		{"other output", lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.NumberIntVal(0)}, lang.AttrStep{Name: "id"}}, false},
		{"not a module", lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.NumberIntVal(0)}}, false},
		{"unknown key of a splat", lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.DynamicVal}, lang.AttrStep{Name: "url"}}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origin := LocalOrigin{
				Addr:        tc.addr,
				Constraints: OriginConstraints{{OfScopeId: lang.ScopeId("module"), OfType: cty.String}},
			}
			if got := output.Matches(origin); got != tc.matches {
				t.Fatalf("expected match=%t, got %t (address %s)", tc.matches, got, origin.Address())
			}
			if len(origin.Addr) != len(tc.addr) {
				t.Fatal("Address must not modify the origin")
			}
		})
	}
}

func TestLocalOrigin_Address_resourceInstanceKey(t *testing.T) {
	root := func(name string) lang.RootStep { return lang.RootStep{Name: name} }
	attr := func(name string) lang.AttrStep { return lang.AttrStep{Name: name} }
	idx := func(key cty.Value) lang.IndexStep { return lang.IndexStep{Key: key} }

	testCases := []struct {
		name     string
		addr     lang.Address
		expected string
	}{
		{"counted resource", lang.Address{root("aws_instance"), attr("web"), idx(cty.NumberIntVal(0)), attr("id")}, "aws_instance.web.id"},
		{"resource with for_each", lang.Address{root("aws_instance"), attr("web"), idx(cty.StringVal("a"))}, "aws_instance.web"},
		{"counted data source", lang.Address{root("data"), attr("aws_ami"), attr("x"), idx(cty.NumberIntVal(1)), attr("id")}, "data.aws_ami.x.id"},
		{"ephemeral resource", lang.Address{root("ephemeral"), attr("random_password"), attr("p"), idx(cty.NumberIntVal(0)), attr("result")}, "ephemeral.random_password.p.result"},
		{"index into a resource attribute", lang.Address{root("aws_instance"), attr("web"), attr("tags"), idx(cty.StringVal("k"))}, `aws_instance.web.tags["k"]`},
		{"index into a variable", lang.Address{root("var"), attr("list"), idx(cty.NumberIntVal(0)), attr("id")}, "var.list[0].id"},
		{"index into a local", lang.Address{root("local"), attr("m"), idx(cty.StringVal("k"))}, `local.m["k"]`},
		{"index into each.value", lang.Address{root("each"), attr("value"), idx(cty.NumberIntVal(0))}, "each.value[0]"},
		{"index into a data source attribute", lang.Address{root("data"), attr("aws_ami"), attr("x"), attr("ids"), idx(cty.NumberIntVal(0))}, "data.aws_ami.x.ids[0]"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origin := LocalOrigin{Addr: tc.addr}
			if got := origin.Address().String(); got != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestOrigins_AtPos_innermost(t *testing.T) {
	rng := func(start, end int) hcl.Range {
		return hcl.Range{
			Filename: "test.tf",
			Start:    hcl.Pos{Line: 1, Column: start + 1, Byte: start},
			End:      hcl.Pos{Line: 1, Column: end + 1, Byte: end},
		}
	}
	// attr = module.app[var.key].url
	outer := LocalOrigin{
		Addr:  lang.Address{lang.RootStep{Name: "module"}, lang.AttrStep{Name: "app"}, lang.IndexStep{Key: cty.DynamicVal}, lang.AttrStep{Name: "url"}},
		Range: rng(7, 30),
	}
	key := LocalOrigin{
		Addr:  lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "key"}},
		Range: rng(18, 25),
	}
	// a path origin with the same range as the key is not held by it
	implied := PathOrigin{Range: rng(18, 25)}
	origins := Origins{outer, key, implied}

	testCases := []struct {
		name     string
		byte     int
		expected Origins
	}{
		{"on the module call", 10, Origins{outer}},
		{"on the key", 22, Origins{key, implied}},
		{"on the output", 28, Origins{outer}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := origins.AtPos("test.tf", hcl.Pos{Line: 1, Column: tc.byte + 1, Byte: tc.byte})
			if !ok {
				t.Fatal("expected an origin")
			}
			if diff := cmp.Diff(tc.expected, got, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("unexpected origins: %s", diff)
			}
		})
	}
}
