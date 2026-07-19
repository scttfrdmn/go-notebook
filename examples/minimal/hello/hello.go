//go:notebook
//
// hello — the whole model on one screen.
//
// A parameterless cell whose result is consumed is an INPUT; a cell that takes a
// parameter is DERIVED. The edge exists because celsius produces `c` and
// fahrenheit takes `c`: a named result feeds any parameter of the same name and
// type. Nothing else is wired.
//
//	go tool notebook run ./examples/minimal/hello
//
// Demonstrates: the input -> derived edge, wiring by name+type. Runs in the
// browser (WASM) and as a native/headless binary. See docs/quickstart.html.

package hello

// Temperature in Celsius.
//
//notebook:slider min=-40 max=100 step=1
func celsius() (c float64) { return 20 }

// Temperature in Fahrenheit — wired in by the parameter name `c`.
func fahrenheit(c float64) (f float64) { return c*9/5 + 32 }
