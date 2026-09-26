// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reference

import (
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl/v2"
)

// LocalOrigin represents a resolved reference origin (traversal)
// targeting a *local* attribute or a block within the same path
type LocalOrigin struct {
	// Addr describes the resolved address of the reference
	Addr lang.Address

	// Range represents the range of the traversal
	Range hcl.Range

	// Constraints represents any traversal expression constraints
	// for the attribute where the origin was found.
	//
	// Further matching against decoded reference targets is needed
	// for >1 constraints, which is done later at runtime as
	// targets and origins can be decoded at different times.
	Constraints OriginConstraints
}

func (lo LocalOrigin) Copy() Origin {
	return LocalOrigin{
		Addr:        lo.Addr.Copy(),
		Range:       lo.Range,
		Constraints: lo.Constraints.Copy(),
	}
}

func (LocalOrigin) isOriginImpl() originSigil {
	return originSigil{}
}

func (lo LocalOrigin) OriginRange() hcl.Range {
	return lo.Range
}

func (lo LocalOrigin) OriginConstraints() OriginConstraints {
	return lo.Constraints
}

func (lo LocalOrigin) AppendConstraints(oc OriginConstraints) MatchableOrigin {
	lo.Constraints = append(lo.Constraints, oc...)
	return lo
}

// Address returns the address the origin is matched against targets
// with. An instance key of a module call is left out, so that
// module.app[0].url and module.app["a"].url match module.app.url, the
// target of the output, as module.app.url does.
func (lo LocalOrigin) Address() lang.Address {
	return withoutModuleInstanceKey(lo.Addr)
}

// withoutModuleInstanceKey removes the index step that follows the call
// name in module.<call>[<key>]...
func withoutModuleInstanceKey(addr lang.Address) lang.Address {
	if len(addr) < 3 {
		return addr
	}
	root, ok := addr[0].(lang.RootStep)
	if !ok || root.Name != "module" {
		return addr
	}
	if _, ok := addr[1].(lang.AttrStep); !ok {
		return addr
	}
	if _, ok := addr[2].(lang.IndexStep); !ok {
		return addr
	}
	out := make(lang.Address, 0, len(addr)-1)
	out = append(out, addr[:2]...)
	return append(out, addr[3:]...)
}
