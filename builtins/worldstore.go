package builtins

import (
	dbstore "barn/db/store"
	"barn/types"
)

// worldstore.go — SPIKE: the typed world-store seam (Candidate A, consumer-side typing).
//
// Topology rationale (see reports/world-seam-topology-spike.md):
//
//   - types.TaskContext.Store stays `interface{}` because making it the concrete
//     *dbstore.Store would force `types` to import `db/store`, and `db/store`
//     already imports `types` => import cycle (hard constraint 1 violated).
//   - Making it a typed INTERFACE in `types` (Candidate B) removes the cycle but
//     turns every store read into dynamic dispatch (hard constraint 2 violated).
//   - `builtins` ALREADY imports `db/store`, so the typing can happen here at the
//     point of use with ZERO new import edges and ZERO dynamic dispatch: the helper
//     returns the CONCRETE *dbstore.Store, so all subsequent method calls are
//     static (direct) calls the compiler can inline.
//
// This file is the single seam: convert `ctx.Store.(*dbstore.Store)` sites to
// `storeFromCtx(ctx)`.

// storeFromCtx is the typed world-store seam. It returns the concrete
// *dbstore.Store so that all reads through it are static dispatch (free reads,
// inlinable), satisfying hard constraint 2. ok is false when no store is wired.
//
// NOTE: there is no `//go:inline` pragma in Go; inlining is the compiler's
// cost-based decision. This helper is small enough that the inliner inlines it at
// every call site (verified via `go build -gcflags=-m`).
func storeFromCtx(ctx *types.TaskContext) (*dbstore.Store, bool) {
	s, ok := ctx.Store.(*dbstore.Store)
	return s, ok
}
