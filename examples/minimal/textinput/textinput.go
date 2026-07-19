//go:notebook
//
// textinput — a string input.
//
// A parameterless cell whose result is a string renders as a text box. Downstream
// cells recompute as you type.
//
//	go tool notebook run ./examples/minimal/textinput
//
// Demonstrates: string input -> text box. See docs/reference-controls.html.

package textinput

// Your name.
func name() (who string) { return "world" }

// A greeting, derived from the name.
func greeting(who string) (msg string) { return "Hello, " + who + "!" }
