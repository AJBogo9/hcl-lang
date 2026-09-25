// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/reference"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestReferenceOriginsTargetingPos_onReference(t *testing.T) {
	dirPath := t.TempDir()
	path := lang.Path{Path: dirPath}

	rng := func(file string, line, col, byte, endCol int) hcl.Range {
		return hcl.Range{
			Filename: file,
			Start:    hcl.Pos{Line: line, Column: col, Byte: byte},
			End:      hcl.Pos{Line: line, Column: endCol, Byte: byte + endCol - col},
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

	// variables.tf
	//   variable "project" {}                      (line 1)
	// locals.tf
	//   locals {
	//     name_prefix = "${var.project}-app"         (line 2)
	//     other       = var.project                  (line 3)
	//   }
	// main.tf
	//   prefix = local.name_prefix                   (line 2)
	//   provider = local.secondary                   (line 3)
	// versions.tf
	//   provider "local" {}                           (line 1)
	//   provider "local" { alias = "secondary" }      (line 2)
	varProjectRng := rng("variables.tf", 1, 1, 0, 22)
	namePrefixRng := rng("locals.tf", 2, 3, 11, 40)
	providerRng := rng("versions.tf", 1, 1, 0, 20)
	secondaryRng := rng("versions.tf", 2, 1, 21, 42)

	pathCtx := &PathContext{
		ReferenceTargets: reference.Targets{
			{Addr: addr("var", "project"), ScopeId: "variable", Type: cty.String, RangePtr: varProjectRng.Ptr(), DefRangePtr: varProjectRng.Ptr()},
			{Addr: addr("local", "name_prefix"), ScopeId: "local", Type: cty.DynamicPseudoType, RangePtr: namePrefixRng.Ptr()},
			{Addr: addr("local"), ScopeId: "provider", Type: cty.DynamicPseudoType, RangePtr: providerRng.Ptr()},
			{Addr: addr("local", "secondary"), ScopeId: "provider", Type: cty.DynamicPseudoType, RangePtr: secondaryRng.Ptr()},
		},
		ReferenceOrigins: reference.Origins{
			reference.LocalOrigin{Addr: addr("var", "project"), Range: rng("locals.tf", 2, 20, 28, 31), Constraints: dynamic},
			reference.LocalOrigin{Addr: addr("var", "project"), Range: rng("locals.tf", 3, 17, 55, 28), Constraints: dynamic},
			reference.LocalOrigin{Addr: addr("local", "name_prefix"), Range: rng("main.tf", 2, 12, 20, 29), Constraints: dynamic},
			reference.LocalOrigin{Addr: addr("local", "secondary"), Range: rng("main.tf", 3, 14, 45, 29), Constraints: reference.OriginConstraints{{OfScopeId: "provider"}}},
		},
	}

	testCases := []struct {
		name            string
		filename        string
		pos             hcl.Pos
		expectedOrigins ReferenceOrigins
	}{
		{
			"on a reference inside a declaration",
			"locals.tf",
			hcl.Pos{Line: 2, Column: 25, Byte: 33},
			ReferenceOrigins{
				{Path: path, Range: rng("locals.tf", 2, 20, 28, 31)},
				{Path: path, Range: rng("locals.tf", 3, 17, 55, 28)},
			},
		},
		{
			"on the enclosing declaration itself",
			"locals.tf",
			hcl.Pos{Line: 2, Column: 5, Byte: 13},
			ReferenceOrigins{
				{Path: path, Range: rng("main.tf", 2, 12, 20, 29)},
			},
		},
		{
			"provider named local is not referenced by local values",
			"versions.tf",
			hcl.Pos{Line: 1, Column: 5, Byte: 4},
			ReferenceOrigins{},
		},
		{
			"on a provider alias reference",
			"main.tf",
			hcl.Pos{Line: 3, Column: 20, Byte: 51},
			ReferenceOrigins{
				{Path: path, Range: rng("main.tf", 3, 14, 45, 29)},
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			d := NewDecoder(&testPathReader{
				paths: map[string]*PathContext{dirPath: pathCtx},
			})
			origins := d.ReferenceOriginsTargetingPos(path, tc.filename, tc.pos)

			if diff := cmp.Diff(tc.expectedOrigins, origins, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatch of reference origins: %s", diff)
			}
		})
	}
}

func TestReferenceOriginsTargetingPos_stepUnderCursor(t *testing.T) {
	dirPath := t.TempDir()
	path := lang.Path{Path: dirPath}

	src := `resource "aws_instance" "web" {}
output "a" { value = aws_instance.web.id }
output "b" { value = aws_instance.web.arn }
`
	f, diags := hclsyntax.ParseConfig([]byte(src), "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}

	rng := func(line, col, byte, endCol int) hcl.Range {
		return hcl.Range{
			Filename: "main.tf",
			Start:    hcl.Pos{Line: line, Column: col, Byte: byte},
			End:      hcl.Pos{Line: line, Column: endCol, Byte: byte + endCol - col},
		}
	}
	addr := func(steps ...string) lang.Address {
		a := lang.Address{lang.RootStep{Name: steps[0]}}
		for _, s := range steps[1:] {
			a = append(a, lang.AttrStep{Name: s})
		}
		return a
	}
	blockRng := rng(1, 1, 0, 33)
	idOrigin := rng(2, 22, 54, 41)
	arnOrigin := rng(3, 22, 97, 42)

	pathCtx := &PathContext{
		Files: map[string]*hcl.File{"main.tf": f},
		ReferenceTargets: reference.Targets{
			{
				Addr:     addr("aws_instance", "web"),
				ScopeId:  "resource",
				Type:     cty.Object(map[string]cty.Type{"id": cty.String, "arn": cty.String}),
				RangePtr: blockRng.Ptr(),
				NestedTargets: reference.Targets{
					{Addr: addr("aws_instance", "web", "id"), ScopeId: "resource", Type: cty.String, RangePtr: blockRng.Ptr()},
					{Addr: addr("aws_instance", "web", "arn"), ScopeId: "resource", Type: cty.String, RangePtr: blockRng.Ptr()},
				},
			},
		},
		ReferenceOrigins: reference.Origins{
			reference.LocalOrigin{Addr: addr("aws_instance", "web", "id"), Range: idOrigin, Constraints: reference.OriginConstraints{{OfType: cty.String}}},
			reference.LocalOrigin{Addr: addr("aws_instance", "web", "arn"), Range: arnOrigin, Constraints: reference.OriginConstraints{{OfType: cty.String}}},
		},
	}

	testCases := []struct {
		name            string
		pos             hcl.Pos
		expectedOrigins ReferenceOrigins
	}{
		{
			"on the resource name: every use of the resource",
			hcl.Pos{Line: 2, Column: 36, Byte: 68},
			ReferenceOrigins{{Path: path, Range: idOrigin}, {Path: path, Range: arnOrigin}},
		},
		{
			"on the resource type: every use of the resource",
			hcl.Pos{Line: 2, Column: 25, Byte: 57},
			ReferenceOrigins{{Path: path, Range: idOrigin}, {Path: path, Range: arnOrigin}},
		},
		{
			"on the attribute: uses of that attribute",
			hcl.Pos{Line: 2, Column: 39, Byte: 71},
			ReferenceOrigins{{Path: path, Range: idOrigin}},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			d := NewDecoder(&testPathReader{
				paths: map[string]*PathContext{dirPath: pathCtx},
			})
			origins := d.ReferenceOriginsTargetingPos(path, "main.tf", tc.pos)

			if diff := cmp.Diff(tc.expectedOrigins, origins, ctydebug.CmpOptions); diff != "" {
				t.Fatalf("mismatch of reference origins: %s", diff)
			}
		})
	}
}
