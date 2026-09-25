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

// A provider named "local" is a type-unaware (dynamic) target at "local",
// which also matches every local.* address under it.
var (
	providerLocal = Target{
		Addr:     lang.Address{lang.RootStep{Name: "local"}},
		ScopeId:  lang.ScopeId("provider"),
		Type:     cty.DynamicPseudoType,
		RangePtr: &hcl.Range{Filename: "versions.tf", Start: hcl.Pos{Line: 1, Column: 1, Byte: 0}, End: hcl.Pos{Line: 1, Column: 20, Byte: 19}},
	}
	providerLocalSecondary = Target{
		Addr:     lang.Address{lang.RootStep{Name: "local"}, lang.AttrStep{Name: "secondary"}},
		ScopeId:  lang.ScopeId("provider"),
		Type:     cty.DynamicPseudoType,
		RangePtr: &hcl.Range{Filename: "versions.tf", Start: hcl.Pos{Line: 3, Column: 1, Byte: 21}, End: hcl.Pos{Line: 5, Column: 2, Byte: 60}},
	}
	localNamePrefix = Target{
		Addr:     lang.Address{lang.RootStep{Name: "local"}, lang.AttrStep{Name: "name_prefix"}},
		ScopeId:  lang.ScopeId("local"),
		Type:     cty.DynamicPseudoType,
		RangePtr: &hcl.Range{Filename: "locals.tf", Start: hcl.Pos{Line: 2, Column: 3, Byte: 11}, End: hcl.Pos{Line: 2, Column: 30, Byte: 38}},
	}
	varNetwork = Target{
		Addr:     lang.Address{lang.RootStep{Name: "var"}, lang.AttrStep{Name: "network"}},
		ScopeId:  lang.ScopeId("variable"),
		Type:     cty.DynamicPseudoType,
		RangePtr: &hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 1, Column: 1, Byte: 0}, End: hcl.Pos{Line: 3, Column: 2, Byte: 40}},
	}
)

func localOrigin(line int, scope lang.ScopeId, steps ...string) LocalOrigin {
	addr := lang.Address{lang.RootStep{Name: steps[0]}}
	for _, s := range steps[1:] {
		addr = append(addr, lang.AttrStep{Name: s})
	}
	return LocalOrigin{
		Addr: addr,
		Range: hcl.Range{
			Filename: "main.tf",
			Start:    hcl.Pos{Line: line, Column: 1, Byte: line * 100},
			End:      hcl.Pos{Line: line, Column: 20, Byte: line*100 + 19},
		},
		Constraints: OriginConstraints{{OfScopeId: scope, OfType: cty.DynamicPseudoType}},
	}
}

func TestTargets_Match_mostSpecific(t *testing.T) {
	testCases := []struct {
		name            string
		targets         Targets
		origin          MatchableOrigin
		expectedTargets Targets
	}{
		{
			"local value wins over provider named local",
			Targets{providerLocal, providerLocalSecondary, localNamePrefix},
			localOrigin(1, "", "local", "name_prefix"),
			Targets{localNamePrefix},
		},
		{
			"provider alias wins over the default configuration",
			Targets{providerLocal, providerLocalSecondary, localNamePrefix},
			localOrigin(2, "provider", "local", "secondary"),
			Targets{providerLocalSecondary},
		},
		{
			"default provider configuration",
			Targets{providerLocal, providerLocalSecondary, localNamePrefix},
			localOrigin(3, "provider", "local"),
			Targets{providerLocal},
		},
		{
			"dynamic target still matches a longer address",
			Targets{providerLocal, varNetwork},
			localOrigin(4, "", "var", "network", "zones"),
			Targets{varNetwork},
		},
		{
			"undeclared local falls back to the only (dynamic) match",
			Targets{providerLocal, localNamePrefix},
			localOrigin(5, "", "local", "undeclared"),
			Targets{providerLocal},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			targets, _ := tc.targets.Match(tc.origin)
			if diff := cmp.Diff(tc.expectedTargets, targets, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatch of targets: %s", diff)
			}
		})
	}
}

func TestOrigins_MatchResolved(t *testing.T) {
	path := lang.Path{Path: t.TempDir(), LanguageID: "opentofu"}
	allTargets := Targets{providerLocal, providerLocalSecondary, localNamePrefix, varNetwork}
	origins := Origins{
		localOrigin(1, "", "local", "name_prefix"),
		localOrigin(2, "provider", "local", "secondary"),
		localOrigin(3, "provider", "local"),
		localOrigin(4, "", "var", "network", "zones"),
		localOrigin(5, "", "local", "undeclared"),
	}

	testCases := []struct {
		name            string
		target          Target
		expectedOrigins Origins
	}{
		{
			"provider named local is not referenced by local values",
			providerLocal,
			Origins{origins[2], origins[4]},
		},
		{
			"aliased provider",
			providerLocalSecondary,
			Origins{origins[1]},
		},
		{
			"local value",
			localNamePrefix,
			Origins{origins[0]},
		},
		{
			"attribute of a dynamic variable",
			varNetwork,
			Origins{origins[3]},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			matched := origins.MatchResolved(path, tc.target, path, func() Targets { return allTargets })
			if diff := cmp.Diff(tc.expectedOrigins, matched, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatch of origins: %s", diff)
			}
		})
	}
}
