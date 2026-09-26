// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestMergedBlockBodySchema(t *testing.T) {
	blockSchema := &schema.BlockSchema{
		Labels: []*schema.LabelSchema{{Name: "type", IsDepKey: true}, {Name: "name"}},
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {IsOptional: true, Constraint: schema.AnyExpression{OfType: cty.Number}},
			},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{Labels: []schema.LabelDependent{{Index: 0, Value: "local_file"}}}): {
				Attributes: map[string]*schema.AttributeSchema{
					"filename": {IsRequired: true, Constraint: schema.AnyExpression{OfType: cty.String}},
				},
			},
		},
	}

	testCases := []struct {
		src   string
		attrs string
	}{
		{`resource "local_file" "f" {}`, "count,filename"},
		{`resource "other" "f" {}`, "count"},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f, diags := hclsyntax.ParseConfig([]byte(tc.src), "main.tf", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatal(diags)
			}
			block := f.Body.(*hclsyntax.Body).Blocks[0]
			merged := MergedBlockBodySchema(block, blockSchema)
			names := make([]string, 0)
			for name := range merged.Attributes {
				names = append(names, name)
			}
			sort.Strings(names)
			if got := strings.Join(names, ","); got != tc.attrs {
				t.Fatalf("expected attributes %s, got %s", tc.attrs, got)
			}
		})
	}
}
