package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

func cli(t *testing.T, args ...string) []byte {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := Main(args, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, errOut.String())
	}
	if errOut.Len() > 0 {
		t.Fatalf("%v wrote to stderr: %q", args, errOut.String())
	}
	return out.Bytes()
}

// showRow is one row of table show's answer.
type showRow struct {
	Noun  string            `json:"noun"`
	ID    string            `json:"id"`
	Data  map[string]string `json:"data"`
	Cells []string          `json:"cells"`
}

func TestTableShowJSON(t *testing.T) {
	// A bare --json is accepted: the output is JSON either way.
	out := cli(t, "table", "show", cities, "--json")
	var rows []showRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err, string(out))
	}
	if len(rows) != 10 || rows[0].Noun != "row" || rows[0].ID != "row/1" || rows[0].Cells[0] != "Tokyo" || rows[5].Data["notes"] != `Banking, "chocolate", lakes` {
		t.Fatalf("show: %+v", rows)
	}
	// Data keeps the header's key order.
	if i, j := bytes.Index(out, []byte(`"city": "Tokyo"`)), bytes.Index(out, []byte(`"country": "Japan"`)); i < 0 || j < i {
		t.Fatalf("key order:\n%s", out)
	}
	// A cut window says which rows it holds on stderr; ids stay file rows.
	var stdout, stderr bytes.Buffer
	args := []string{"table", "show", cities, "--where", "country~peru", "--sort", "population", "--desc", "--columns", "city,population", "--limit", "1"}
	if err := Main(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err, stderr.String())
	}
	rows = nil
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Data["city"] != "Lima" || rows[0].ID != "row/4" || len(rows[0].Cells) != 2 || stderr.String() != "status: rows 1–1 of 2\n" {
		t.Fatalf("filtered show: %s\nstderr %q", stdout.String(), stderr.String())
	}
	out = cli(t, "table", "show", "testdata/scores.tsv", "--search", "bo", "--no_header")
	rows = nil
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Data["B"] != "bo" {
		t.Fatalf("no-header search: %s", out)
	}
}

// --format prints table show's rows as the CSV's own table: the shown
// columns over the cells, numbers right-aligned.
func TestTableShowFormats(t *testing.T) {
	want := map[string]string{
		"table":    "city   population\nTokyo    13960000\nOsaka     2750000\n",
		"markdown": "| city | population |\n| --- | ---: |\n| Tokyo | 13960000 |\n| Osaka | 2750000 |\n",
		"toon":     "row[2]{city,population}:\n  Tokyo,13960000\n  Osaka,2750000\n",
	}
	for format, text := range want {
		var stdout, stderr bytes.Buffer
		if err := Main([]string{"table", "show", cities, "--columns", "city,population", "--limit", "2", "--format", format}, strings.NewReader(""), &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		if stdout.String() != text {
			t.Errorf("--format %s:\n%s\nwant\n%s", format, stdout.String(), text)
		}
	}
	out := cli(t, "table", "columns", cities, "--format", "toon")
	if !strings.HasPrefix(string(out), "[7]{index,name,type}:\n  0,city,string\n") {
		t.Fatalf("columns --format toon:\n%s", out)
	}
}

func TestTableColumnsAndStats(t *testing.T) {
	var cols []ColumnInfo
	if err := json.Unmarshal(cli(t, "table", "columns", "testdata/parts.csv"), &cols); err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 || cols[2].Name != "price" || cols[2].Type != "float" {
		t.Fatalf("columns: %+v", cols)
	}
	var s Stats
	if err := json.Unmarshal(cli(t, "table", "stats", "testdata/hosts.psv"), &s); err != nil {
		t.Fatal(err)
	}
	if s.Rows != 2 || s.Columns != 3 || s.Delim != "pipe" || s.Each[2].Type != "bool" {
		t.Fatalf("stats: %+v", s)
	}
	var errOut bytes.Buffer
	if err := Main([]string{"table", "stats", "testdata/missing.csv"}, strings.NewReader(""), &bytes.Buffer{}, &errOut); err == nil {
		t.Fatal("a missing file is not an error")
	}
	if !strings.Contains(errOut.String(), "table.stats failed") {
		t.Fatalf("a failed job's line was dropped: %q", errOut.String())
	}
}

func TestCatalogDescribesTheTableNoun(t *testing.T) {
	var doc struct {
		Name  string `json:"name"`
		Nouns []struct {
			Name  string `json:"name"`
			Verbs []struct {
				Name       string   `json:"name"`
				Positional []string `json:"positional"`
			} `json:"verbs"`
		} `json:"nouns"`
	}
	if err := json.Unmarshal(cli(t, "catalog"), &doc); err != nil {
		t.Fatal(err)
	}
	verbs := map[string]bool{}
	for _, n := range doc.Nouns {
		if n.Name == "table" {
			for _, v := range n.Verbs {
				verbs[v.Name] = true
			}
		}
	}
	if doc.Name != "o-csv" || !verbs["show"] || !verbs["columns"] || !verbs["stats"] {
		t.Fatalf("catalog: %+v", doc)
	}
}

// The catalog carries the viewer's nouns, verbs and key rows, not only
// table: the edit verbs register up front, so a host sees them. On the
// command line no sheet is open and nothing is in the grid, so they refuse.
func TestCatalogCarriesTheViewerVerbs(t *testing.T) {
	var doc ocli.CatalogDoc
	if err := json.Unmarshal(cli(t, "catalog"), &doc); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, n := range doc.Nouns {
		for _, v := range n.Verbs {
			have[n.Name+"."+v.Name] = true
		}
	}
	for _, key := range []string{"table.show", "sheet.save", "sheet.edit", "cell.edit", "cell.transform", "row.transform", "column.transform", "ochazuke.selection.transform", "transformations.list"} {
		if !have[key] {
			t.Errorf("the catalog has no %s", key)
		}
	}
	rows := map[string]bool{}
	for _, b := range doc.Bindings {
		rows[b.Noun+" "+b.Mode+" "+b.Keys+" "+b.Request.Key()] = true
	}
	for _, row := range []string{"row normal t u row.transform", "column normal t s column.transform", "ochazuke.grid visual-block s column.sort"} {
		if !rows[row] {
			t.Errorf("the catalog has no key row %q", row)
		}
	}
	if len(doc.Pages) != 0 {
		t.Errorf("the command line declares pages %+v; the sheet page is the viewer's", doc.Pages)
	}
	sh := newShell(nil, io.Discard, io.Discard)
	for _, q := range []ochazuke.Request{
		{Noun: "sheet", Verb: "save"}, {Noun: "sheet", Verb: "addColumn", Args: ochazuke.Arguments{"name": "x"}},
		{Noun: "cell", Verb: "transform", Args: ochazuke.Arguments{"name": "upper"}},
		{Noun: "row", Verb: "transform", Args: ochazuke.Arguments{"name": "upper"}},
		{Noun: "column", Verb: "transform", Args: ochazuke.Arguments{"name": "upper"}},
	} {
		if _, err := sh.Runtime.Execute(q); err == nil {
			t.Errorf("%s ran on the command line", q.Key())
		}
	}
	// Turning editing on does not open a sheet: the edit verbs still refuse.
	if _, err := sh.Runtime.Execute(ochazuke.Request{Noun: "sheet", Verb: "edit", Args: ochazuke.Arguments{"on": true}}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []ochazuke.Request{{Noun: "sheet", Verb: "save"}, {Noun: "sheet", Verb: "addColumn", Args: ochazuke.Arguments{"name": "x"}}, {Noun: "sheet", Verb: "undo"}} {
		if _, err := sh.Runtime.Execute(q); err == nil {
			t.Errorf("%s ran with editing on and no sheet", q.Key())
		}
	}
}

// The delimiter setting is the default --delim of the table verbs, and
// config export writes both settings.
func TestSettingsReachTheCommandLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.txt")
	if err := os.WriteFile(path, []byte("a,b\tc\n1,2\t3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	sh := newShell(strings.NewReader(""), &out, &errOut)
	if err := sh.Runtime.Settings().Set(settingDelimiter, "tab"); err != nil {
		t.Fatal(err)
	}
	if err := sh.Run(context.Background(), []string{"table", "columns", path}); err != nil {
		t.Fatal(err, errOut.String())
	}
	var cols []ColumnInfo
	if err := json.Unmarshal(out.Bytes(), &cols); err != nil || len(cols) != 2 || cols[0].Name != "a,b" {
		t.Fatalf("columns with delimiter=tab: %v %s", err, out.String())
	}
	if err := sh.Runtime.Settings().Set(settingDelimiter, "two"); err == nil {
		t.Fatal("a two-character delimiter was accepted")
	}
	dir := t.TempDir()
	if _, err := sh.Runtime.ExportConfig(dir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ochazuke.SettingsReferenceFile))
	if err != nil || !strings.Contains(string(data), `"delimiter"`) || !strings.Contains(string(data), `"column-width"`) {
		t.Fatalf("settings reference: %v\n%s", err, data)
	}
}
