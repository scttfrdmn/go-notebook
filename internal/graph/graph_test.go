package graph

import (
	"testing"
)

// mkCell is a terse constructor for tests. params and results are given as
// name/type pairs; a result named "err" of type "error" is marked IsError.
func mkCell(id string, line int, params, results [][2]string) *Cell {
	c := &Cell{
		ID:    CellID(id),
		Pos:   Position{Filename: "n.go", Line: line, Column: 1},
		Label: id,
	}
	for i, p := range params {
		kind := Wired
		switch p[1] {
		case "context.Context":
			kind = Injected
		}
		c.Params = append(c.Params, Param{
			Name: Symbol(p[0]), Type: p[1], Kind: kind,
			Pos: Position{Filename: "n.go", Line: line, Column: 10 + i},
		})
	}
	for i, r := range results {
		c.Results = append(c.Results, Result{
			Name: Symbol(r[0]), Type: r[1], IsError: r[1] == "error",
			Pos: Position{Filename: "n.go", Line: line, Column: 40 + i},
		})
	}
	return c
}

// buildGraph adds cells in order and indexes them.
func buildGraph(cells ...*Cell) *Graph {
	g := New()
	for _, c := range cells {
		g.Add(c)
	}
	g.Index()
	return g
}

func TestParamKindString(t *testing.T) {
	tests := []struct {
		k    ParamKind
		want string
	}{
		{Wired, "wired"},
		{Injected, "injected"},
		{Delayed, "delayed"},
		{ParamKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("ParamKind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestIndexProducer(t *testing.T) {
	g := buildGraph(
		mkCell("arrivalRate", 1, nil, [][2]string{{"lambda", "PerHour"}}),
		mkCell("offeredLoad", 2, [][2]string{{"lambda", "PerHour"}}, [][2]string{{"a", "Erlangs"}}),
	)
	if got := g.Producer["lambda"]; got != "arrivalRate" {
		t.Errorf("producer of lambda = %q, want arrivalRate", got)
	}
	if got := g.Producer["a"]; got != "offeredLoad" {
		t.Errorf("producer of a = %q, want offeredLoad", got)
	}
}

func TestErrorResultIsNotAnEdge(t *testing.T) {
	// A trailing error result must not appear in the producer index.
	g := buildGraph(
		mkCell("load", 1, nil, [][2]string{{"rows", "[]Row"}, {"err", "error"}}),
	)
	if _, ok := g.Producer["err"]; ok {
		t.Error("error result was indexed as a producer; it must be the failure channel")
	}
	if g.Producer["rows"] != "load" {
		t.Error("named data result should still be indexed")
	}
}

func TestCheckClean(t *testing.T) {
	// A well-formed diamond produces no diagnostics.
	g := buildGraph(
		mkCell("a", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("b", 2, [][2]string{{"x", "int"}}, [][2]string{{"y", "int"}}),
		mkCell("c", 3, [][2]string{{"x", "int"}}, [][2]string{{"z", "int"}}),
		mkCell("d", 4, [][2]string{{"y", "int"}, {"z", "int"}}, [][2]string{{"w", "int"}}),
	)
	if diags := g.Check(); len(diags) != 0 {
		t.Errorf("expected clean graph, got %d diagnostics:\n%v", len(diags), diags)
	}
}

func TestCheckMissingProducer(t *testing.T) {
	g := buildGraph(
		mkCell("offeredLoad", 1, nil, [][2]string{{"a", "Erlangs"}}),
		mkCell("utilization", 2, [][2]string{{"a", "Erlangs"}, {"c", "int"}}, [][2]string{{"rho", "float64"}}),
	)
	diags := g.Check()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d:\n%v", len(diags), diags)
	}
	d := diags[0]
	if d.Pos.Line != 2 {
		t.Errorf("diagnostic should point at the consuming param on line 2, got line %d", d.Pos.Line)
	}
	wantMsg := "cell \"utilization\" needs `c int`, but no cell produces it."
	if d.Msg != wantMsg {
		t.Errorf("msg = %q, want %q", d.Msg, wantMsg)
	}
}

func TestMissingProducerSuggestsSameType(t *testing.T) {
	// A near-miss: the consumer wants `a Erlangs`, and a cell produces that
	// type under a different name. The hint should suggest it.
	g := buildGraph(
		mkCell("offeredLoad", 1, nil, [][2]string{{"load", "Erlangs"}}),
		mkCell("utilization", 2, [][2]string{{"a", "Erlangs"}}, [][2]string{{"rho", "float64"}}),
	)
	diags := g.Check()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d:\n%v", len(diags), diags)
	}
	if diags[0].Hint == "" || diags[0].HintPos == nil {
		t.Errorf("expected a same-type suggestion hint, got %+v", diags[0])
	}
	wantHint := "Did you mean `offeredLoad`, which produces `Erlangs`?"
	if diags[0].Hint != wantHint {
		t.Errorf("hint = %q, want %q", diags[0].Hint, wantHint)
	}
}

func TestCheckDuplicateProducer(t *testing.T) {
	g := buildGraph(
		mkCell("first", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("second", 2, nil, [][2]string{{"x", "int"}}),
	)
	diags := g.Check()
	if len(diags) == 0 {
		t.Fatal("expected a duplicate-producer diagnostic")
	}
	found := false
	for _, d := range diags {
		if d.Pos.Line == 2 && d.HintPos != nil && d.HintPos.Line == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic on line 2 pointing back at line 1, got:\n%v", diags)
	}
}

func TestCheckTypeMismatch(t *testing.T) {
	// Same symbol name, different type: the edge must not silently connect.
	g := buildGraph(
		mkCell("producer", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("consumer", 2, [][2]string{{"x", "string"}}, [][2]string{{"y", "int"}}),
	)
	diags := g.Check()
	if len(diags) != 1 {
		t.Fatalf("expected 1 type-mismatch diagnostic, got %d:\n%v", len(diags), diags)
	}
	if diags[0].HintPos == nil || diags[0].HintPos.Line != 1 {
		t.Errorf("mismatch diagnostic should point back at the producer on line 1, got %+v", diags[0])
	}
}

func TestInjectedParamIsNotAnEdge(t *testing.T) {
	// A context.Context parameter has no producer and must not be reported as
	// a missing edge.
	g := buildGraph(
		mkCell("fetch", 1, [][2]string{{"ctx", "context.Context"}}, [][2]string{{"data", "[]byte"}}),
	)
	if diags := g.Check(); len(diags) != 0 {
		t.Errorf("injected context param should not produce diagnostics, got:\n%v", diags)
	}
}
