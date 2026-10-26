package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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

func TestTableShowJSON(t *testing.T) {
	// A bare --json is accepted: the output is JSON either way.
	out := cli(t, "table", "show", cities, "--json")
	var res struct {
		Columns []string            `json:"columns"`
		Total   int                 `json:"total"`
		Rows    []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err, string(out))
	}
	if res.Total != 10 || len(res.Rows) != 10 || res.Columns[0] != "city" || res.Rows[5]["notes"] != `Banking, "chocolate", lakes` {
		t.Fatalf("show: %+v", res)
	}
	// Rows keep the header's key order.
	if i, j := bytes.Index(out, []byte(`"city": "Tokyo"`)), bytes.Index(out, []byte(`"country": "Japan"`)); i < 0 || j < i {
		t.Fatalf("key order:\n%s", out)
	}
	out = cli(t, "table", "show", cities, "--where", "country~peru", "--sort", "population", "--desc", "--columns", "city,population", "--limit", "1")
	res.Rows = nil
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || len(res.Rows) != 1 || res.Rows[0]["city"] != "Lima" || len(res.Rows[0]) != 2 {
		t.Fatalf("filtered show: %s", out)
	}
	out = cli(t, "table", "show", "testdata/scores.tsv", "--search", "bo", "--no_header")
	res.Rows = nil
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["B"] != "bo" {
		t.Fatalf("no-header search: %s", out)
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
