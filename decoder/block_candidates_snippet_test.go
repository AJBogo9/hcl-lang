// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl-lang/schema"
)

func TestSnippetForBlock_dependencyKeyLabels(t *testing.T) {
	resource := &schema.BlockSchema{Labels: []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}}
	// a module call's name is a dependency key, so that two calls of one
	// source get their own targets, but the user names it
	module := &schema.BlockSchema{Labels: []*schema.LabelSchema{
		{Name: "name", IsDepKey: true},
	}}
	output := &schema.BlockSchema{Labels: []*schema.LabelSchema{
		{Name: "name"},
	}}

	testCases := []struct {
		blockType string
		block     *schema.BlockSchema
		prefill   bool
		expected  string
	}{
		{"resource", resource, false, "resource \"${1}\" \"${2:name}\" {\n  ${3}\n}"},
		{"module", module, false, "module \"${1:name}\" {\n  ${2}\n}"},
		{"output", output, false, "output \"${1:name}\" {\n  ${2}\n}"},
		{"resource", resource, true, "resource \"${0}\" \"name\" {\n}"},
		{"module", module, true, "module \"${1:name}\" {\n  ${2}\n}"},
		{"output", output, true, "output \"${1:name}\" {\n  ${2}\n}"},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s-prefill-%t", i, tc.blockType, tc.prefill), func(t *testing.T) {
			if got := snippetForBlock(tc.blockType, tc.block, tc.prefill); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
