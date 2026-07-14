# Exploration doc — agent-native notebooks

*Status: `needs-conversation`. No milestone. No implementation issues. Scott's words: "potentially interesting but I would want to talk it through more rather than just 'it does agents'." This doc respects that exactly — it explores a property and asks whether it is a real thing or a rationalization of a property we happen to have.*

---

## Claim vs. evidence

**Evidence:** three properties exist and are exercised today.
- A notebook is a **plain Go file** — no JSON envelope, no live kernel.
- `notebook check` emits **structured diagnostics** (the analyzer's output).
- `notebook run --headless --json` emits **structured, provenance-stamped output** (`design.md:304`, and the content-addressed `--json` from KC12).

**Claim:** that these three add up to a genuinely agent-native loop — that an agent can author → check → run → read → iterate with **no UI and no live kernel** — and that this is *materially* better for an agent than the alternatives, rather than a property any compiled language with JSON output also has. That second half is the claim this doc is most suspicious of.

---

## The observation

Because a notebook is a file you write and a binary you run, the agent loop is:

```
write file → notebook check → notebook run --headless --json → parse JSON → edit file → repeat
```

Contrast:
- **`.ipynb`** is JSON-wrapped *state* an agent must surgically edit (cells, outputs, execution counts interleaved) — and then still needs a live kernel to execute.
- **marimo** is a cleaner file, but still needs a live Python environment to run.
- **Ours** is a file you write with ordinary text edits and a binary you run to completion, whose diagnostics and outputs are already machine-readable.

The loop needs no editor integration, no kernel protocol, no session. That *shape* is real. Whether it's a *differentiator* is the open question.

---

## The questions this thread has to answer (before it could ever be framed)

1. **What is the loop, end to end, with real commands?** Write it out concretely against an existing example (`capacity`, `taxi`) — the exact `check` invocation, the exact `--json` shape the agent parses, the exact edit it makes in response to a diagnostic. If it can't be written concretely, it isn't understood yet.

2. **Is `check`'s diagnostic output structured enough to *act on*?** Not just "is it JSON" — does a diagnostic name the cell, the leaf, the type, the fix-shaped information an agent needs to edit the file correctly? Or is it prose an agent has to guess at? (This is the same question the project keeps asking of its own outputs: a specification is a claim; does the diagnostic actually carry what a consumer needs, or does it only *look* structured?)

3. **Is `--json` self-describing enough to interpret without the source?** An agent that wrote the file has the source. But can it interpret the output from the JSON alone — cell names, MIME types, provenance — or does interpretation require re-reading the Go? If the JSON is self-describing, that's a real property; if not, name the gap.

4. **What would a falsifiable kill criterion even look like?** Candidate: *given only `check` and `run --headless --json`, an agent takes a research question and produces a working notebook that answers it, with no human touching the file.* **Is that the right test, or is it a demo dressed as a claim?** A single scripted success proves nothing (the KC4 error in a new suit); a rate across varied questions might. This doc does **not** adopt that KC — it flags that even the KC is unsettled, which is the honest state.

5. **What is the honest case *against*?** State it plainly: **any language with a compiler and a JSON-emitting CLI could support this loop.** Go is not special here; Rust, TypeScript-with-a-build, even a C program with structured output could all do `write → build → run → parse`. If that's the whole story, "agent-native" is just "compiled + structured output," which the project already has for other reasons and which is not a feature worth a milestone. The *possible* differentiator — worth talking through, not asserting — is that a go-notebook file is simultaneously (a) a valid Go package the agent's normal toolchain understands, (b) a reactive graph whose structure the agent can query without running it, and (c) a reproducible artifact. Whether that trio is more than the sum of "compiled + JSON" is exactly what needs a conversation, not a build.

---

## Why this is not ready

Every framing round in this project has begun by picking up something on momentum and building it before the human decision was made. This is a candidate to do that with, and it must not be. The property is real; its *value* is unestablished; and the kill criterion for it is itself unsettled. Those are three separate unknowns, and a milestone would paper over all three. A doc and a set of questions is the correct output. Talk it through first.
