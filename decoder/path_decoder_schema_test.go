// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"testing"

	"github.com/hashicorp/hcl-lang/schema"
)

func TestPathDecoder_Schema(t *testing.T) {
	bodySchema := &schema.BodySchema{
		Attributes: map[string]*schema.AttributeSchema{
			"name": {IsRequired: true},
		},
	}
	testCases := []struct {
		name     string
		pathCtx  *PathContext
		expected *schema.BodySchema
	}{
		{"with schema", &PathContext{Schema: bodySchema}, bodySchema},
		{"without schema", &PathContext{}, nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := testPathDecoder(t, tc.pathCtx)
			if got := d.Schema(); got != tc.expected {
				t.Fatalf("expected %p, got %p", tc.expected, got)
			}
		})
	}
	if got := (&PathDecoder{}).Schema(); got != nil {
		t.Fatalf("expected nil schema without a path context, got %p", got)
	}
}
