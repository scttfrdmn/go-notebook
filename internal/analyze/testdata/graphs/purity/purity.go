//go:notebook
//
// A fixture for purity classification. Some cells are genuinely impure (they
// reach math/rand or time.Now); one is pure arithmetic that merely formats with
// fmt (CHA conservatively calls it impure — the safe over-approximation); one
// is a trivial pure constant.

package purity

import (
	"fmt"
	"math/rand"
	"time"
)

// A pure constant.
func base() (n int) { return 7 }

// noise is genuinely impure: it reaches math/rand. It must NEVER be classified
// pure — a cached stale draw would be silently wrong.
func noise(n int) (r int) { return n + rand.Intn(100) }

// stamp is genuinely impure: it reaches time.Now.
func stamp(n int) (t int64) { return int64(n) + time.Now().Unix() }

// formatted is pure arithmetic, but formats with fmt.Sprintf. CHA cannot prove
// fmt's interface dispatch stays pure, so it is conservatively impure — costing
// only a cache hit.
func formatted(n int) (s string) { return fmt.Sprintf("value=%d", n*2) }

// doubled is pure: arithmetic only, no fmt, no impure reach.
func doubled(n int) (d int) { return n * 2 }
