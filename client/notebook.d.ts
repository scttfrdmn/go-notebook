// TypeScript declarations for the go-notebook JS client (notebook.js).
//
// These are hand-written to match notebook.js exactly — there is no build step
// and no toolchain in this repo on purpose (importless notebooks are the ethos).
// A TS consumer gets full types; a plain-JS consumer ignores this file.

export interface WidgetColumn {
  Name: string;
  Type: string;
}

export interface WidgetMeta {
  /** the control category, derived from the leaf's Go type */
  Kind: "range" | "select" | "multi" | "bool" | "draggable" | "table" | string;
  /** a table leaf's grid columns; absent for every other kind */
  Columns?: WidgetColumn[];
}

export interface LeafType {
  /** the type as declared ("PerHour", "int") */
  Name: string;
  /** the basic kind it resolves to; absent for a composite/interface leaf */
  Underlying?: "int" | "float64" | "bool" | "string" | string;
}

export interface CellMeta {
  ID: string;
  /** the leaf symbol this cell produces, if it is an input control */
  Leaf?: string;
  Label?: string;
  Directives?: Record<string, string>;
  /** upstream producer cell ids (the dependency-graph edges) */
  In?: string[];
  /** verbatim cell source */
  Source?: string;
  /** static control descriptor; present only for leaves */
  Widget?: WidgetMeta;
  /** the leaf's Go result type; present only for leaves */
  Type?: LeafType;
}

export interface WireEvent {
  epoch: number;
  /** the cell id; empty on the wave-settled marker */
  cell: string;
  state: "running" | "done" | "error" | "blocked" | "stale" | "settled";
  mime?: string;
  data?: string;
  err?: string;
}

export interface ValueEvent {
  /** the wave this value belongs to; absent on a port that predates it */
  epoch?: number;
  /** the cell whose value this is; absent on a settled marker */
  cell?: string;
  /** the cell's typed value, flattened to a plain JS value; absent on a settled marker */
  value?: unknown;
  /** true on the terminal wave-settled marker: every value for `epoch` has arrived */
  settled?: boolean;
}

/** The raw globalThis.notebook object the WASM build publishes. */
export interface NotebookPort {
  meta: CellMeta[];
  provenance: unknown;
  layout: string[][] | null;
  set(leaf: string, value: unknown): void;
  subscribe(fn: (ev: WireEvent) => void): () => void;
  subscribeValues?(fn: (ev: ValueEvent) => void): () => void;
  values(): Record<string, unknown>;
  start(): void;
}

/** A notebook input leaf, as read from meta. */
export interface Leaf {
  /** the leaf symbol you pass to set() */
  symbol: string;
  /** the cell id that produces it */
  cell: string;
  label: string;
  /** the widget kind, or null for a bare scalar */
  kind: string | null;
  /** a table leaf's columns, else [] */
  columns: WidgetColumn[];
  /** the leaf's Go result type, or null if the port predates it */
  type: LeafType | null;
}

/** Wrap a raw notebook port (or globalThis.notebook) in the structural client. */
export function connect(port?: NotebookPort): Notebook;

/** The structural client over a go-notebook WASM port. */
export class Notebook {
  constructor(port: NotebookPort);
  readonly port: NotebookPort;
  /** the capabilities this port supports, derived from the port (never a lie) */
  capabilities(): string[];
  /** whether the port supports a named capability (feature detection, no try/catch) */
  can(cap: string): boolean;
  /** every cell's metadata */
  cells(): CellMeta[];
  /** the input leaves — the settable surface */
  leaves(): Leaf[];
  /** the dependency graph as { cell -> upstream producer cells } */
  graph(): Record<string, string[]>;
  /** set a leaf (crosses the port's coercer like a UI edit) */
  set(leaf: string, value: unknown): void;
  /** pull a snapshot of every leaf's current value */
  values(): Record<string, unknown>;
  /** subscribe to rendered events (mime/data) */
  subscribe(fn: (ev: WireEvent) => void): () => void;
  /** subscribe to typed value events ({epoch, cell, value}); throws on an older port */
  subscribeValues(fn: (ev: ValueEvent) => void): () => void;
  /** coherent per-wave value snapshots: fn({epoch, values}) once per settled wave */
  subscribeEpoch(fn: (snapshot: { epoch: number; values: Record<string, unknown> }) => void): () => void;
  /** run the first wave */
  start(): void;
}
