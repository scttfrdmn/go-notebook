//go:notebook
//
// cancel — an expensive cell that respects cancellation.
//
// A cell that asks for a context.Context parameter gets one injected — it is the
// one parameter with no upstream cell. When an input changes before a slow cell
// finishes, the scheduler cancels the in-flight computation and only the newest
// result is shown. Ask for ctx and honor ctx.Done() and rapid slider drags stay
// responsive instead of queueing stale work.
//
//	go tool notebook run ./examples/minimal/cancel
//
// Demonstrates: context.Context injection, cancellable recompute.

package cancel

import "context"

// How many iterations of (deliberately) expensive work.
//
//notebook:slider min=1 max=40 step=1
func iterations() (n int) { return 10 }

// A slow sum that checks for cancellation between chunks. If the slider moves
// again mid-flight, ctx is cancelled and this returns early; the scheduler
// discards the stale result and runs the new one.
func result(ctx context.Context, n int) (sum float64) {
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			return sum // cancelled — abandon this stale computation
		}
		sum += busy(1_000_000)
	}
	return sum
}

// busy is an ordinary helper (unnamed return) — CPU work standing in for a real
// expensive computation. Pure arithmetic, so the cell stays WASM-portable.
func busy(steps int) float64 {
	x := 0.0
	for i := 0; i < steps; i++ {
		x += float64(i%7) * 0.5
	}
	return x
}
