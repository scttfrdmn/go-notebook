//go:notebook
//
// A 3D surface on the GPU — and the framework boundary, drawn explicitly.
//
// Every other notebook in this corpus obeys the rule that makes the project what
// it is: the view is a pure readout of Go values, and direct manipulation is one
// Go cell (a grip), never JavaScript. This notebook is the deliberate exception,
// and it exists to show *where that rule ends*.
//
// WebGL has no Go form. To spin a shaded surface on the GPU you need a <canvas>, a
// shader pair, and a draw loop — all JavaScript, running in the browser. The design
// doc names this exact case: "drawing canvases, gamepads, webcams … are genuinely
// arbitrary input and need a raw HTML/JS escape hatch. That escape hatch IS a
// framework surface. Quarantine it and say so." This file is that quarantine.
//
// The seam is still honest, and that is the point of putting it here rather than
// pretending it doesn't exist:
//
//   - **Go owns the math.** `surface` computes the heightfield z = f(x, y) — a pure
//     function of the sliders, the interesting part, the part you'd write in Go
//     anyway. Change a slider and Go recomputes the field; the wave runs through
//     the graph exactly like every other notebook.
//   - **The GPU owns the pixels.** The injected JavaScript reads that heightfield,
//     builds a triangle mesh, and draws it with a rotating camera. It computes no
//     science; it is a renderer.
//
// One mechanism worth seeing plainly: the browser does NOT execute a <script> tag
// inserted via innerHTML (the client sets body.innerHTML to our HTML). It DOES run
// an inline event handler. So the bootstrap rides on `<img onerror>` — arbitrary
// JS in an attribute, which is precisely the "framework surface" being described.
// No other notebook needs this trick; this one advertises it.

package surface

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Grid resolution — the surface is res×res samples. Higher is smoother and heavier
// (the heightfield crosses to the browser, so this is also the payload-size knob).
//
//notebook:slider min=16 max=64 step=4
func resolution() (res int) { return 40 }

// Amplitude of the surface, in tenths (12 → 1.2). How tall the ripples stand.
//
//notebook:slider min=2 max=20 step=1
func amplitudeTenths() (amp int) { return 10 }

// Spatial frequency — how many ripples across the sheet.
//
//notebook:slider min=1 max=8 step=1
func frequency() (freq int) { return 3 }

// ---------------------------------------------------------------------------
// Compute (Go) — the math, pure.
// ---------------------------------------------------------------------------

// The heightfield z = f(x, y) over a res×res grid on [-1,1]². A radial ripple —
// amplitude·sin(freq·π·r)·falloff — chosen because it reads as a clear 3D shape
// once it's spinning. Pure: a function of the three sliders alone, so scrubbing is
// exact and the cell caches. This is the whole scientific content; everything
// downstream is rendering.
func surface(res int, amp int, freq int) (field Heightfield) {
	a := float64(amp) / 10
	f := float64(freq)
	z := make([]float64, res*res)
	for j := 0; j < res; j++ {
		for i := 0; i < res; i++ {
			x := 2*float64(i)/float64(res-1) - 1
			y := 2*float64(j)/float64(res-1) - 1
			r := hypot(x, y)
			z[j*res+i] = a * sin(f*pi*r) * falloff(r)
		}
	}
	return Heightfield{Res: res, Z: z}
}

// ---------------------------------------------------------------------------
// View (the quarantined escape hatch) — HTML + WebGL.
// ---------------------------------------------------------------------------

// The surface, on the GPU. Returns a value that renders to text/html: a canvas and
// an onerror-bootstrapped WebGL program that draws the heightfield as a rotating,
// height-shaded mesh. This is the framework surface — the only cell in the corpus
// that emits JavaScript. Drag a slider and Go recomputes the field; the mesh
// rebuilds from the new numbers.
//
//notebook:height=480
func scene(field Heightfield) (gpu Scene) {
	return Scene{Field: field}
}

// A 3D surface on the GPU — and the framework boundary, drawn explicitly.
func intro() (md Markdown) {
	return `This is the corpus's **deliberate exception**. Every other notebook keeps
the view a pure readout of Go values and does direct manipulation in one Go cell.
WebGL has no Go form, so this one drops to raw HTML and JavaScript — the escape
hatch the design doc says to *quarantine and label*.

The seam is still honest: **Go computes the heightfield** (the math, pure, in the
graph); **the GPU draws it** (the injected JavaScript is a renderer and computes no
science). Drag the sliders — Go recomputes, the mesh rebuilds.

One detail on show: a ` + "`<script>`" + ` inserted via innerHTML does not run, but an
inline ` + "`onerror`" + ` does, so the WebGL bootstrap rides on an image handler — arbitrary
JS in an attribute, which is exactly the framework surface being described.`
}

// ===========================================================================
// Math helpers (stdlib-free tiny versions keep the cell graph obviously pure)
// ===========================================================================

const pi = 3.141592653589793

func hypot(x, y float64) float64 { return sqrt(x*x + y*y) }

func falloff(r float64) float64 {
	if r >= 1 {
		return 0
	}
	return (1 - r) * (1 - r)
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	g := x
	for range 20 {
		g = 0.5 * (g + x/g)
	}
	return g
}

// sin via range-reduced Taylor series — enough for a smooth surface, and keeps the
// cell provably pure (no import that a call-graph walk has to reason about).
func sin(x float64) float64 {
	// reduce to [-pi, pi]
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	term := x
	sum := x
	x2 := x * x
	for n := 1; n < 10; n++ {
		term *= -x2 / float64((2*n)*(2*n+1))
		sum += term
	}
	return sum
}

// ===========================================================================
// Types
// ===========================================================================

// Heightfield is the res×res grid of z values — the pure output of the math,
// row-major, x fastest.
type Heightfield struct {
	Res int
	Z   []float64
}

// Scene wraps a heightfield and renders it to a WebGL canvas.
type Scene struct {
	Field Heightfield
}

// Render emits the text/html the client injects: a <canvas> and an <img onerror>
// that bootstraps WebGL. The heightfield rides in a data- attribute as a JSON
// array; the handler reads it, builds a triangle mesh, and animates a rotating,
// height-shaded draw. The Go side never touches a pixel; the JS never touches the
// math. That the JS lives in a string here — not in a .js file, not importable, not
// typechecked — is the honest cost of the escape hatch, and why it's quarantined
// to this one cell.
func (s Scene) Render() Rendered {
	// Serialize the heightfield as a compact JSON number array for the handler.
	var z strings.Builder
	z.WriteByte('[')
	for i, v := range s.Field.Z {
		if i > 0 {
			z.WriteByte(',')
		}
		z.WriteString(strconv.FormatFloat(v, 'g', 5, 64))
	}
	z.WriteByte(']')

	// The bootstrap is one JS function invoked from onerror. It's kept in a single
	// place, escaped once for the attribute. res and the data ride in dataset.
	var b strings.Builder
	fmt.Fprintf(&b, `<canvas id="surf" width="720" height="440" `+
		`style="width:100%%;max-width:720px;display:block;background:#0f1524;border-radius:8px" `+
		`data-res="%d" data-z='%s'></canvas>`, s.Field.Res, escapeAttr(z.String()))
	// The image never loads (src=""), so onerror fires immediately — on every
	// re-render, because the client replaces innerHTML each wave, so a fresh img is
	// inserted and the GL re-inits from the new heightfield.
	fmt.Fprintf(&b, `<img alt="" src="" style="display:none" onerror="%s">`, escapeAttr(bootstrapJS))
	return Rendered{MIME: "text/html", Data: b.String()}
}

// bootstrapJS is the WebGL renderer, invoked from the img onerror. It reads the
// canvas dataset (res + heightfield), builds a triangle mesh with per-vertex height,
// compiles a minimal shader pair, and animates a rotating camera. Pure rendering —
// it computes no part of the surface, only how to show it. This string is the
// framework surface: raw JS, no types, no Go tooling over it.
const bootstrapJS = `
var cv=document.getElementById('surf');
if(cv&&cv.getContext){
 var gl=cv.getContext('webgl');
 if(gl){
  var R=+cv.dataset.res, Z=JSON.parse(cv.dataset.z);
  // Build triangle vertices (x,y,z) over the grid from the heightfield.
  var V=[];
  for(var j=0;j<R-1;j++)for(var i=0;i<R-1;i++){
   var x0=2*i/(R-1)-1,x1=2*(i+1)/(R-1)-1,y0=2*j/(R-1)-1,y1=2*(j+1)/(R-1)-1;
   var a=Z[j*R+i],b=Z[j*R+i+1],c=Z[(j+1)*R+i],d=Z[(j+1)*R+i+1];
   V.push(x0,y0,a, x1,y0,b, x0,y1,c,  x1,y0,b, x1,y1,d, x0,y1,c);
  }
  var buf=gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER,buf);
  gl.bufferData(gl.ARRAY_BUFFER,new Float32Array(V),gl.STATIC_DRAW);
  var vs='attribute vec3 p;uniform mat4 m;varying float h;void main(){h=p.z;gl_Position=m*vec4(p,1.0);}';
  var fs='precision mediump float;varying float h;void main(){float t=clamp(h*0.6+0.5,0.0,1.0);gl_FragColor=vec4(0.15+0.7*t,0.2+0.4*t,0.9-0.5*t,1.0);}';
  function sh(t,s){var o=gl.createShader(t);gl.shaderSource(o,s);gl.compileShader(o);return o;}
  var pr=gl.createProgram();gl.attachShader(pr,sh(gl.VERTEX_SHADER,vs));gl.attachShader(pr,sh(gl.FRAGMENT_SHADER,fs));
  gl.linkProgram(pr);gl.useProgram(pr);
  var pl=gl.getAttribLocation(pr,'p');gl.enableVertexAttribArray(pl);gl.vertexAttribPointer(pl,3,gl.FLOAT,false,0,0);
  var ml=gl.getUniformLocation(pr,'m');
  gl.enable(gl.DEPTH_TEST);gl.clearColor(0.06,0.08,0.14,1.0);
  function mul(a,b){var o=[];for(var r=0;r<4;r++)for(var c=0;c<4;c++){var s=0;for(var k=0;k<4;k++)s+=a[k*4+r]*b[c*4+k];o[c*4+r]=s;}return o;}
  function persp(){var f=1.0/Math.tan(0.6),n=0.1,fa=10.0;return [f/1.6,0,0,0, 0,f,0,0, 0,0,(fa+n)/(n-fa),-1, 0,0,2*fa*n/(n-fa),0];}
  // stop any previous loop when the cell re-renders
  if(window.__surfRAF)cancelAnimationFrame(window.__surfRAF);
  function frame(t){
   var an=t*0.0005;
   var ca=Math.cos(an),sa=Math.sin(an);
   var rot=[ca,0,-sa,0, 0,1,0,0, sa,0,ca,0, 0,0,0,1];
   var tilt=[1,0,0,0, 0,0.7,0.7,0, 0,-0.7,0.7,0, 0,0,0,1];
   var trans=[1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0.15,-3.4,1];
   var mv=mul(trans,mul(tilt,rot));
   var m=mul(persp(),mv);
   gl.uniformMatrix4fv(ml,false,new Float32Array(m));
   gl.viewport(0,0,cv.width,cv.height);
   gl.clear(gl.COLOR_BUFFER_BIT|gl.DEPTH_BUFFER_BIT);
   gl.drawArrays(gl.TRIANGLES,0,V.length/3);
   window.__surfRAF=requestAnimationFrame(frame);
  }
  window.__surfRAF=requestAnimationFrame(frame);
 }
}
`

// escapeAttr escapes a string for a double-quoted or single-quoted HTML attribute.
// Both quote kinds and & are escaped so the value can't break out regardless of
// which delimiter the attribute uses (data-z uses single quotes, onerror double).
func escapeAttr(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
