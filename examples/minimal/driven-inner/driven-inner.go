// driven-inner — an outer notebook that DRIVES an embedded inner notebook: its
// own knob pushes values into the inner one's leaves, treating the inner notebook
// as a parameterized black-box compute. Builds on notebook-in-notebook (a static
// embedded figure) by adding the drive direction via the JS client's connectFrame.
//
// Still composition the ALLOWED way (the notebook as a component, not an import):
// the inner notebook is its own compiled program; the outer reaches it only
// through its one public port (set/subscribe), same-origin, over the iframe. No
// merged graph. The outer notebook's cell can't hold a live JS reference, so it
// smuggles a tiny bootstrap via the text/html render escape hatch (an <img
// onerror>, the gpulife pattern) that imports connectFrame and wires the knob.
//
//	go tool notebook build --target=wasm -o ./out ./examples/minimal/driven-inner
//	go tool notebook build --target=wasm -o ./out/inner ./examples/capacity
//	cp client/notebook.js ./out/notebook-client.js   # the bootstrap imports it
//	(cd out && python3 -m http.server)
//
// Demonstrates: connectFrame (drive an embedded notebook), the escape-hatch JS
// bridge, composition-as-component. See docs/reference-js-client.html.
//
//go:notebook
package driveninner

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }

func intro() (heading Markdown) {
	return "## Driving a black box\n\n" +
		"The inner notebook below is a compute with knobs. **This** notebook owns the " +
		"knob — drag it and the value is pushed into the inner notebook's `c` leaf, " +
		"which recomputes. The outer document decides the parameter; the inner one computes."
}

// Panel renders the outer knob, the inner notebook as an embedded figure, and a
// bootstrap that bridges the two via connectFrame. The knob's oninput calls a
// function the bootstrap installs once the inner port is reachable.
type Panel struct{ Src string }

func (p Panel) Render() Rendered {
	// The bridge JS: dynamically import the client, connectFrame the inner iframe,
	// expose a drive fn. connectFrame owns the readiness poll + same-origin port
	// grab, so this is a few honest lines, not a hand-rolled poll.
	//
	// It rides an <img onerror>, NOT a <script> tag: the client injects a cell's
	// text/html via innerHTML, and innerHTML-inserted <script> does NOT execute —
	// but an <img onerror> DOES fire (the gpulife escape-hatch pattern). import()
	// is a function, so it works inside the handler with no module context.
	boot := `import('./notebook-client.js').then(async m => {` +
		`const inner = await m.connectFrame(document.getElementById('inner'));` +
		`window.__drive = v => inner.set('c', v);` +
		`document.getElementById('bridged').textContent = 'bridged — knob drives inner c';` +
		`}).catch(e => { document.getElementById('bridged').textContent = 'bridge failed: ' + e.message; })`
	return Rendered{MIME: "text/html", Data: `<div style="margin:.5rem 0">` +
		`<input type="range" min="1" max="256" value="80" ` +
		`oninput="window.__drive && window.__drive(Number(this.value))"> servers (outer knob)</div>` +
		`<div id="bridged" style="font:12px monospace;color:#5b6472;margin:.3rem 0">bridging…</div>` +
		`<iframe id="inner" src="` + p.Src + `" title="inner compute" ` +
		`style="width:100%;height:520px;border:1px solid #e7ebf0;border-radius:10px"></iframe>` +
		`<img alt="" src="" style="display:none" onerror="` + escapeAttr(boot) + `">`}
}

// escapeAttr escapes a string for use inside a double-quoted HTML attribute — the
// bridge JS rides an onerror= attribute, so its quotes and angle brackets must be
// entity-encoded or the markup breaks. (A hand-rolled escape that MISSES a case is
// exactly what broke an early prototype; this covers the four that matter here.)
func escapeAttr(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			out = append(out, "&quot;"...)
		case '&':
			out = append(out, "&amp;"...)
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func panel() (view Panel) { return Panel{Src: "inner/?embed"} }
