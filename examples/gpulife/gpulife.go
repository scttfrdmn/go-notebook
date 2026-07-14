//go:notebook
//
// Life on the GPU — the parallel dividend, back in the tab.
//
// The turing notebook states a caveat out loud: its grid update is embarrassingly
// parallel — the textbook case for the scheduler's goroutine fan-out — but that
// fan-out is "real natively and absent in the [WASM] tab," because GOOS=js is
// single-threaded. The parallelism the design brags about is exactly the thing the
// browser tier gives up.
//
// This notebook is the answer to that caveat. It runs Conway's Game of Life — the
// same shape of computation, a neighbourhood update over a big grid — but on a
// WebGPU **compute shader**. The parallelism is back, in the tab, on the GPU
// instead of goroutines. Turn the grid up to 512×512: a quarter-million cells
// stepping many times a second, in a browser, with no server.
//
// This is the corpus's SECOND framework-surface notebook (surface was the first),
// and like surface it says so plainly. WebGPU has no Go form; the compute shader and
// the render pipeline are WGSL and JavaScript, emitted as a string and run by the
// browser — the escape hatch the design doc quarantines. The seam is still honest:
//
//   - **Go owns the seed.** initial() computes the starting grid — a pure function
//     of the size, density, and seed sliders (a small deterministic LCG, no global
//     RNG, so it's reproducible and cacheable, exactly like every other cell).
//   - **The GPU owns the iteration.** The WGSL compute shader reads the current grid
//     from a storage buffer and writes the next generation to another, one GPU
//     thread per cell. JavaScript computes no Life rules; it schedules the GPU.
//
// So the interesting, checkable part is Go, in the graph; the massively-parallel
// part is the GPU. The bootstrap rides on `<img onerror>` for the same reason
// surface's does: a <script> inserted via innerHTML does not run, an inline handler
// does. Here it's an async handler, because acquiring a WebGPU device is async.

package gpulife

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Grid size — the world is n×n cells. Crank it up: the GPU shrugs at sizes that
// would crawl on a single CPU thread, which is the whole point.
//
//notebook:slider min=64 max=512 step=64
func size() (n int) { return 256 }

// Initial live density, in percent. How much of the seed grid starts alive.
//
//notebook:slider min=5 max=60 step=5
func density() (pct int) { return 30 }

// Seed for the initial pattern. A leaf, not a global RNG — so the starting grid is
// a pure, reproducible function of the sliders (the corpus's no-hidden-state rule).
//
//notebook:slider min=1 max=999 step=1
func seed() (s int) { return 42 }

// ---------------------------------------------------------------------------
// Compute (Go) — the seed grid, pure.
// ---------------------------------------------------------------------------

// The initial grid: n×n cells, each alive with probability ~pct/100, chosen by a
// deterministic LCG keyed on the seed. Pure — same (n, pct, s) gives the same grid
// every time — so the cell caches and there is no global randomness to make the
// notebook silently unreproducible. Go owns this; the GPU takes it from here.
func initial(n int, pct int, s int) (grid Life) {
	cells := make([]uint8, n*n)
	// A plain 64-bit LCG (Numerical Recipes constants), seeded from the slider.
	state := uint64(s)*2654435761 + 1
	threshold := uint64(pct) * 0xFFFFFFFF / 100
	for i := range cells {
		state = state*6364136223846793005 + 1442695040888963407
		if (state >> 32) < threshold {
			cells[i] = 1
		}
	}
	return Life{N: n, Cells: cells}
}

// ---------------------------------------------------------------------------
// View (the quarantined escape hatch) — WebGPU compute + render.
// ---------------------------------------------------------------------------

// Life, on the GPU. Returns a value that renders to text/html: a canvas plus an
// onerror-bootstrapped WebGPU program that uploads the seed grid, steps Life on a
// compute shader, and draws each generation. This is the framework surface — the
// compute shader and the pipeline are WGSL/JS, not Go. Drag the sliders and Go
// recomputes the seed; the GPU re-runs from it.
//
//notebook:height=560
func scene(grid Life) (gpu Scene) {
	return Scene{Grid: grid}
}

// Life on the GPU — the parallel dividend, back in the tab.
func intro() (md Markdown) {
	return `The **turing** notebook admits its parallel grid update — the goroutine
fan-out the design is proudest of — is *absent in the browser tab*, because
` + "`GOOS=js`" + ` is single-threaded. This notebook is the answer: the same shape of
computation, Conway's Game of Life, run on a WebGPU **compute shader**. The
parallelism is back — in the tab, on the GPU. Turn the grid up to 512 and watch a
quarter-million cells step many times a second.

The seam stays honest. **Go owns the seed grid** (a pure, reproducible function of
the sliders, in the dependency graph); **the GPU owns the iteration** (one thread
per cell, in WGSL). Like the **surface** notebook, this is the framework escape
hatch — WebGPU has no Go form — and it says so: the compute shader is a string the
browser runs, not typed Go.`
}

// ===========================================================================
// Types
// ===========================================================================

// Life is the grid state: N×N cells, row-major, 1 = alive. This is what Go computes
// and hands to the GPU; the GPU never hands it back (the iteration stays on-device).
type Life struct {
	N     int
	Cells []uint8
}

// Scene wraps the seed grid and renders it to a WebGPU canvas.
type Scene struct {
	Grid Life
}

// Render emits the text/html the client injects: a <canvas> carrying the seed grid
// and size in dataset, plus an <img onerror> that async-bootstraps WebGPU. The grid
// rides as a compact string of '0'/'1' (one char per cell) — far smaller than JSON
// for a 512×512 boolean field. The handler uploads it, runs the compute shader, and
// draws. That the WGSL lives in a Go string — untyped, un-tooled, run by the browser
// — is the honest cost of the escape hatch, quarantined to this one cell.
func (s Scene) Render() Rendered {
	// Pack the grid as a run of '0'/'1' — compact and trivial to parse in JS.
	var cells strings.Builder
	cells.Grow(len(s.Grid.Cells))
	for _, c := range s.Grid.Cells {
		if c == 1 {
			cells.WriteByte('1')
		} else {
			cells.WriteByte('0')
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<canvas id="life" width="560" height="560" `+
		`style="width:100%%;max-width:560px;display:block;background:#0f1524;`+
		`border-radius:8px;image-rendering:pixelated" `+
		`data-n="%d" data-cells="%s"></canvas>`, s.Grid.N, escapeAttr(cells.String()))
	fmt.Fprintf(&b, `<div id="lifemsg" style="font:12px monospace;color:#5b6472;margin-top:.4rem"></div>`)
	// src="" → onerror fires immediately, on every re-render (the client replaces
	// innerHTML each wave), so the GPU re-inits from the fresh seed grid.
	fmt.Fprintf(&b, `<img alt="" src="" style="display:none" onerror="%s">`, escapeAttr(bootstrapJS))
	return Rendered{MIME: "text/html", Data: b.String()}
}

// bootstrapJS is the WebGPU program, invoked from the img onerror as an async IIFE
// (device acquisition is async). It uploads the seed grid to a storage buffer, runs
// a compute shader that steps Life (one invocation per cell, toroidal wrap), pings-
// pongs two buffers, and draws the current buffer with a render pipeline that maps a
// full-screen triangle's fragments back to cells. It computes no Life rules on the
// CPU — the rules live in the WGSL @compute entry point. If WebGPU is unavailable it
// writes an honest message rather than leaving the canvas blank.
const bootstrapJS = `(async()=>{
 var cv=document.getElementById('life'), msg=document.getElementById('lifemsg');
 if(!navigator.gpu){ if(msg)msg.textContent='WebGPU not available in this browser (try Chrome/Edge).'; return; }
 try{
  var adapter=await navigator.gpu.requestAdapter();
  var dev=await adapter.requestDevice();
  var ctx=cv.getContext('webgpu');
  var fmt=navigator.gpu.getPreferredCanvasFormat();
  ctx.configure({device:dev,format:fmt,alphaMode:'opaque'});
  var N=+cv.dataset.n, cellsStr=cv.dataset.cells;
  // seed grid -> Uint32 array (0/1), uploaded to two ping-pong storage buffers.
  var cells=new Uint32Array(N*N);
  for(var i=0;i<N*N;i++) cells[i]=cellsStr.charCodeAt(i)===49?1:0;
  var bytes=cells.byteLength;
  function sbuf(){ return dev.createBuffer({size:bytes,usage:GPUBufferUsage.STORAGE|GPUBufferUsage.COPY_DST|GPUBufferUsage.COPY_SRC}); }
  var bufA=sbuf(), bufB=sbuf();
  dev.queue.writeBuffer(bufA,0,cells);
  var uni=dev.createBuffer({size:4,usage:GPUBufferUsage.UNIFORM|GPUBufferUsage.COPY_DST});
  dev.queue.writeBuffer(uni,0,new Uint32Array([N]));

  // Compute shader: one invocation per cell, reads the 8 neighbours (wrap), writes
  // the next generation. THIS is the parallel step turing's WASM tab can't do.
  var computeWGSL=
   'struct U{n:u32};@group(0) @binding(0) var<uniform> u:U;'+
   '@group(0) @binding(1) var<storage,read> src:array<u32>;'+
   '@group(0) @binding(2) var<storage,read_write> dst:array<u32>;'+
   'fn idx(x:u32,y:u32)->u32{return y*u.n+x;}'+
   '@compute @workgroup_size(8,8) fn main(@builtin(global_invocation_id) g:vec3<u32>){'+
   ' let n=u.n; if(g.x>=n||g.y>=n){return;}'+
   ' var c:u32=0u;'+
   ' for(var dy:i32=-1;dy<=1;dy=dy+1){for(var dx:i32=-1;dx<=1;dx=dx+1){'+
   '  if(dx==0&&dy==0){continue;}'+
   '  let xx=(i32(g.x)+dx+i32(n))%i32(n); let yy=(i32(g.y)+dy+i32(n))%i32(n);'+
   '  c=c+src[idx(u32(xx),u32(yy))];'+
   ' }}'+
   ' let alive=src[idx(g.x,g.y)];'+
   ' dst[idx(g.x,g.y)]=select(select(0u,1u,c==3u),select(0u,1u,c==2u||c==3u),alive==1u);'+
   '}';
  var cmod=dev.createShaderModule({code:computeWGSL});
  var cpipe=dev.createComputePipeline({layout:'auto',compute:{module:cmod,entryPoint:'main'}});
  function cbind(a,b){return dev.createBindGroup({layout:cpipe.getBindGroupLayout(0),entries:[
   {binding:0,resource:{buffer:uni}},{binding:1,resource:{buffer:a}},{binding:2,resource:{buffer:b}}]});}

  // Render: a full-screen triangle; the fragment shader maps its pixel back to a
  // cell and colours alive vs dead. Reads the same storage buffer the compute wrote.
  var renderWGSL=
   'struct U{n:u32};@group(0) @binding(0) var<uniform> u:U;'+
   '@group(0) @binding(1) var<storage,read> grid:array<u32>;'+
   '@vertex fn vs(@builtin(vertex_index) i:u32)->@builtin(position) vec4<f32>{'+
   ' var p=array<vec2<f32>,3>(vec2(-1.0,-1.0),vec2(3.0,-1.0),vec2(-1.0,3.0));'+
   ' return vec4<f32>(p[i],0.0,1.0);}'+
   '@fragment fn fs(@builtin(position) pos:vec4<f32>)->@location(0) vec4<f32>{'+
   ' let n=f32(u.n);'+
   ' let x=u32(clamp(floor(pos.x/560.0*n),0.0,n-1.0));'+
   ' let y=u32(clamp(floor(pos.y/560.0*n),0.0,n-1.0));'+
   ' let a=grid[y*u.n+x];'+
   ' if(a==1u){return vec4<f32>(0.24,0.83,0.53,1.0);}'+
   ' return vec4<f32>(0.06,0.08,0.14,1.0);}';
  var rmod=dev.createShaderModule({code:renderWGSL});
  var rpipe=dev.createRenderPipeline({layout:'auto',vertex:{module:rmod,entryPoint:'vs'},
   fragment:{module:rmod,entryPoint:'fs',targets:[{format:fmt}]},primitive:{topology:'triangle-list'}});
  function rbind(a){return dev.createBindGroup({layout:rpipe.getBindGroupLayout(0),entries:[
   {binding:0,resource:{buffer:uni}},{binding:1,resource:{buffer:a}}]});}

  if(window.__lifeStop)window.__lifeStop=true;
  window.__lifeStop=false;
  var cur=bufA, nxt=bufB, gen=0;
  var wg=Math.ceil(N/8);
  function frame(){
   if(window.__lifeStop)return;
   var enc=dev.createCommandEncoder();
   // a few generations per frame so it visibly evolves
   for(var s=0;s<4;s++){
    var cp=enc.beginComputePass();
    cp.setPipeline(cpipe); cp.setBindGroup(0,cbind(cur,nxt));
    cp.dispatchWorkgroups(wg,wg); cp.end();
    var t=cur; cur=nxt; nxt=t; gen++;
   }
   var rp=enc.beginRenderPass({colorAttachments:[{view:ctx.getCurrentTexture().createView(),
    loadOp:'clear',storeOp:'store',clearValue:{r:0.06,g:0.08,b:0.14,a:1}}]});
   rp.setPipeline(rpipe); rp.setBindGroup(0,rbind(cur)); rp.draw(3); rp.end();
   dev.queue.submit([enc.finish()]);
   if(msg)msg.textContent='WebGPU compute shader · '+N+'×'+N+' cells · generation '+gen;
   requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
 }catch(e){ if(msg)msg.textContent='WebGPU error: '+e.message; }
})()`

// escapeAttr escapes a string for an HTML attribute (either quote kind), so the
// grid payload and the handler can't break out regardless of delimiter. Same helper
// shape as the surface notebook.
func escapeAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s))
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
