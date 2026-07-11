package analyze

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// update regenerates the golden files instead of comparing against them:
//
//	go test ./internal/analyze -run TestGolden -update
var update = flag.Bool("update", false, "rewrite golden files")

// goldenGraph is the JSON shape written to testdata/graphs/*.want.json. It is a
// stable, position-free-ish projection of the graph: enough to pin the wiring,
// labels, kinds, and purity without being brittle to line-number churn in the
// fixtures. Positions are covered by the diagnostics golden tests instead.
type goldenGraph struct {
	Order    []string              `json:"order"`
	Helpers  []string              `json:"helpers,omitempty"`
	Producer map[string]string     `json:"producer"`
	Cells    map[string]goldenCell `json:"cells"`
}

type goldenCell struct {
	Label      string            `json:"label"`
	Pure       bool              `json:"pure"`
	IsLeaf     bool              `json:"isLeaf"`
	Directives map[string]string `json:"directives,omitempty"`
	Params     []goldenParam     `json:"params"`
	Results    []goldenResult    `json:"results"`
}

type goldenParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Kind string `json:"kind"`
}

type goldenResult struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	IsError bool   `json:"isError,omitempty"`
}

func toGolden(g *graph.Graph) goldenGraph {
	out := goldenGraph{
		Order:    make([]string, 0, len(g.Order)),
		Producer: make(map[string]string, len(g.Producer)),
		Cells:    make(map[string]goldenCell, len(g.Cells)),
	}
	for _, id := range g.Order {
		out.Order = append(out.Order, string(id))
	}
	for _, id := range g.Helpers {
		out.Helpers = append(out.Helpers, string(id))
	}
	for sym, id := range g.Producer {
		out.Producer[string(sym)] = string(id)
	}
	for id, c := range g.Cells {
		gc := goldenCell{Label: c.Label, Pure: c.Pure, IsLeaf: c.IsLeaf, Directives: c.Directives}
		for _, p := range c.Params {
			gc.Params = append(gc.Params, goldenParam{
				Name: string(p.Name), Type: p.Type, Kind: p.Kind.String(),
			})
		}
		for _, r := range c.Results {
			gc.Results = append(gc.Results, goldenResult{
				Name: string(r.Name), Type: r.Type, IsError: r.IsError,
			})
		}
		out.Cells[string(id)] = gc
	}
	return out
}

// TestGolden analyzes every notebook under testdata/graphs and compares the
// derived graph against its .want.json. These fixtures include notebooks whose
// features (folds) are not implemented yet — parsing must not regress when
// those features land.
func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob("testdata/graphs/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			g, _, err := TypesAnalyzer{}.Analyze(dir)
			if err != nil {
				t.Fatalf("analyze: %v", err)
			}
			// The golden pins the purity verdict too, so run the (off-hot-path)
			// refinement pass the same way a build would.
			pkg, err := LoadForPurity(dir)
			if err != nil {
				t.Fatalf("load for purity: %v", err)
			}
			RefinePurity(pkg, g)
			got, err := json.MarshalIndent(toGolden(g), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			goldenPath := dir + ".want.json"
			if *update {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden (run with -update to create): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("graph mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					filepath.Base(dir), got, want)
			}
		})
	}
}
