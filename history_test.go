package main

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/unixsuperhero/ochazuke"
	ocli "github.com/unixsuperhero/ochazuke/cli"
	"github.com/unixsuperhero/ochazuke/grid"
)

// frameworkOperations are the operations ochazuke registers on its own for
// the parts o-csv builds on (the runtime, the model, a grid, the picker):
// every other operation is a verb o-csv defines.
func frameworkOperations() map[string]bool {
	r := ochazuke.NewRuntime()
	ochazuke.NewModel(r)
	r.Instance("page", ochazuke.Kind("sheet"))
	if _, err := grid.New(r, "grid", "page", nil, nil); err != nil {
		panic(err)
	}
	ochazuke.NewPicker(r)
	out := map[string]bool{}
	for key := range r.Operations {
		out[key] = true
	}
	return out
}

// Every verb o-csv defines declares its history kind (Operation.HistoryKind)
// rather than leaving it to the default, in the viewer and in the catalog, and
// the catalog's history is that kind. A new verb without one fails here.
func TestEveryVerbDeclaresItsHistory(t *testing.T) {
	framework := frameworkOperations()
	for _, edit := range []bool{false, true} {
		a, err := New(Options{}, edit)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for key, op := range a.r.Operations {
			if framework[key] {
				continue
			}
			n++
			if op.HistoryKind == "" {
				t.Errorf("viewer verb %s declares no history kind", key)
			}
		}
		if n == 0 {
			t.Fatal("the viewer defines no verbs")
		}
	}

	var doc ocli.CatalogDoc
	if err := json.Unmarshal(cli(t, "catalog"), &doc); err != nil {
		t.Fatal(err)
	}
	ops := newShell(nil, io.Discard, io.Discard).Runtime.Operations
	n := 0
	for _, noun := range doc.Nouns {
		for _, v := range noun.Verbs {
			key := noun.Name + "." + v.Name
			if framework[key] {
				continue
			}
			n++
			if v.History == "" || ops[key].HistoryKind == "" || v.History != ops[key].HistoryKind {
				t.Errorf("catalog verb %s: history %q, declared %q", key, v.History, ops[key].HistoryKind)
			}
		}
	}
	if n != 3 {
		t.Fatalf("the catalog has %d o-csv verbs, want the table noun's 3", n)
	}
}

// A spot check of the kinds: reads are fetches, edits changes, view moves
// actions.
func TestVerbHistoryKinds(t *testing.T) {
	a, err := New(Options{}, true)
	if err != nil {
		t.Fatal(err)
	}
	ops := newShell(nil, io.Discard, io.Discard).Runtime.Operations
	for key, want := range map[string]string{
		"table.show": ochazuke.HistoryFetch, "table.columns": ochazuke.HistoryFetch, "table.stats": ochazuke.HistoryFetch,
	} {
		if got := ops[key].HistoryKind; got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]string{
		"sheet.state": ochazuke.HistoryFetch, "transformations.list": ochazuke.HistoryFetch, "sheet.load": ochazuke.HistoryFetch,
		"sheet.save": ochazuke.HistoryChange, "cell.edit": ochazuke.HistoryChange, "column.sort": ochazuke.HistoryChange, "cell.yank": ochazuke.HistoryChange,
		"sheet.move": ochazuke.HistoryAction, "sheet.search": ochazuke.HistoryAction, "row.detail": ochazuke.HistoryAction,
	} {
		if got := a.r.Operations[key].HistoryKind; got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
}
