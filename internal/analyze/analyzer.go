// Package analyze turns notebook source into a [graph.Graph].
//
// The [Analyzer] interface is the seam that lets the source-analysis backend be
// swapped without touching the graph algorithms or the engine. Today the only
// implementation is [TypesAnalyzer], which loads the package with go/packages
// and reads the type checker's results. When re-analysis on every keystroke
// starts to hurt (the largest engineering risk in the design), a gopls-backed
// implementation can replace it behind the same interface.
//
// Everything go/types-specific lives in this package. The graph package it
// produces is plain data.
package analyze

import "github.com/scttfrdmn/go-notebook/internal/graph"

// Analyzer derives a notebook's dependency graph from source.
//
// Analyze returns the graph together with any diagnostics. A non-nil error is
// reserved for failures to load or analyze at all (bad directory, package that
// does not compile at the syntax level); ordinary notebook problems — a missing
// producer, a cycle, an unnamed result — are returned as diagnostics on a graph
// that is otherwise as complete as possible, so the caller can report several
// at once rather than one per run.
type Analyzer interface {
	Analyze(dir string) (*graph.Graph, []graph.Diagnostic, error)
}
