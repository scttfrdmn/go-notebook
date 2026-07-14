package boundary

import (
	"math"
	"strings"
	"testing"
)

func da() Draggable[Pt] { return classA() }
func db() Draggable[Pt] { return classB() }

// TestFitPure confirms the model is a pure function of (points, λ): same inputs →
// identical weights. This is what lets a drag re-fit from scratch and λ scrub both
// ways — no training state carried between frames.
func TestFitPure(t *testing.T) {
	m1 := fit(da(), db(), 10)
	m2 := fit(da(), db(), 10)
	if m1 != m2 {
		t.Fatalf("fit not pure: %+v vs %+v", m1, m2)
	}
}

// TestSeparatesTheClouds: on the well-separated default clouds, the fitted model
// classifies every training point correctly.
func TestSeparatesTheClouds(t *testing.T) {
	m := fit(da(), db(), 10)
	if acc := accuracy(da().Value, db().Value, m); acc < 0.999 {
		t.Errorf("training accuracy %.2f on separable clouds, want 100%%", acc)
	}
}

// TestRegularizationShrinksWeights is the teaching claim (the seatbelt): larger λ
// gives a smaller weight magnitude, monotonically. If this inverts, "L2 stops the
// boundary chasing outliers" is false.
func TestRegularizationShrinksWeights(t *testing.T) {
	mag := func(lam int) float64 {
		m := fit(da(), db(), lam)
		return math.Hypot(m.W0, m.W1)
	}
	lo, mid, hi := mag(2), mag(20), mag(80)
	if !(lo > mid && mid > hi) {
		t.Errorf("|w| did not shrink monotonically with λ: %.3f, %.3f, %.3f", lo, mid, hi)
	}
}

// TestOutlierMovesBoundaryLessUnderRegularization is the footgun-and-seatbelt claim,
// made quantitative: dragging a class-B point deep into A's territory shifts the
// boundary, but far less when regularization is high. We measure the boundary angle
// change caused by the outlier at low vs high λ.
func TestOutlierMovesBoundaryLessUnderRegularization(t *testing.T) {
	outlier := func() Draggable[Pt] {
		b := db()
		pts := append([]Pt(nil), b.Value...)
		pts[0] = Pt{X: 2.5, Y: 3.5} // a red point dropped into the blue cloud
		return Draggable[Pt]{Value: pts}
	}

	shift := func(lam int) float64 {
		clean := angleDeg(fit(da(), db(), lam))
		dirty := angleDeg(fit(da(), outlier(), lam))
		return math.Abs(dirty - clean)
	}
	lowLambda := shift(2)   // weak regularization — outlier should swing the line
	highLambda := shift(80) // strong — the line should barely move

	if lowLambda <= highLambda {
		t.Errorf("outlier did not move the boundary MORE at low λ: low=%.1f° high=%.1f°", lowLambda, highLambda)
	}
}

// TestTwoGripLeaves confirms the surface carries two distinct grip leaves — a blue
// point routes to classA, a red point to classB — the mechanism this notebook
// stresses. Each class's grips address its own leaf, by index.
func TestTwoGripLeaves(t *testing.T) {
	a := classA().WithLeaf("a")
	b := classB().WithLeaf("b")
	s := surface(a, b, fit(a, b, 10))

	if len(s.A) != len(a.Value) || len(s.B) != len(b.Value) {
		t.Fatalf("grip counts wrong: A %d/%d, B %d/%d", len(s.A), len(a.Value), len(s.B), len(b.Value))
	}
	for i, h := range s.A {
		if h.Ref.Leaf != "a" || h.Ref.Index != i {
			t.Errorf("class-A grip %d addresses %s:%d, want a:%d", i, h.Ref.Leaf, h.Ref.Index, i)
		}
	}
	for i, h := range s.B {
		if h.Ref.Leaf != "b" || h.Ref.Index != i {
			t.Errorf("class-B grip %d addresses %s:%d, want b:%d", i, h.Ref.Leaf, h.Ref.Index, i)
		}
	}
}

// TestSurfaceRenders confirms the decision surface renders an SVG carrying the PNG
// heatmap, the boundary line, and grips from both leaves.
func TestSurfaceRenders(t *testing.T) {
	a := classA().WithLeaf("a")
	b := classB().WithLeaf("b")
	data := surface(a, b, fit(a, b, 10)).Render().Data
	for _, want := range []string{"<svg", "data:image/png;base64,", "<line", `data-grip="a:0"`, `data-grip="b:0"`} {
		if !strings.Contains(data, want) {
			t.Errorf("surface SVG missing %q", want)
		}
	}
}
