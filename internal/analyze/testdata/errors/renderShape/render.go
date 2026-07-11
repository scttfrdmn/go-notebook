//go:notebook
//
// A cell whose output has a Render() method with the WRONG shape: the field is
// misspelled Mime (should be MIME). The reflection probe would silently fail to
// render it; check must catch it as a diagnostic instead.

package rendershape

// Rendered here has Mime, not MIME — the classic typo the importless design
// can't catch with the type system, so check catches it.
type Rendered struct {
	Mime string
	Data string
}

// A cell that intends to render but has the wrong field name.
func chart() (c Chart) { return Chart{} }

// Chart renders — but to the mis-shaped Rendered.
type Chart struct{}

func (Chart) Render() Rendered { return Rendered{Mime: "image/svg+xml", Data: "<svg/>"} }
