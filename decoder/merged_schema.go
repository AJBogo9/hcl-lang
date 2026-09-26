// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"github.com/hashicorp/hcl-lang/decoder/internal/schemahelper"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// MergedBlockBodySchema returns the schema of the body of block: the
// static body of blockSchema merged with the dependent body which the
// block's labels and attributes select, such as the arguments of a
// resource type or the inputs of a module call. It is what the decoder
// itself uses for hover, completion and validation.
func MergedBlockBodySchema(block *hclsyntax.Block, blockSchema *schema.BlockSchema) *schema.BodySchema {
	merged, _ := schemahelper.MergeBlockBodySchemas(block.AsHCLBlock(), blockSchema)
	return merged
}
