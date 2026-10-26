package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDetectDelimiter(t *testing.T) {
	for _, c := range []struct {
		file  string
		delim rune
		cols  int
	}{
		{"testdata/cities.csv", ',', 7},
		{"testdata/scores.tsv", '\t', 3},
		{"testdata/parts.csv", ';', 3},
		{"testdata/hosts.psv", '|', 3},
	} {
		tab, err := ReadFile(c.file, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if tab.Delim != c.delim || len(tab.Header) != c.cols {
			t.Errorf("%s: delim %q cols %d, want %q %d", c.file, tab.Delim, len(tab.Header), c.delim, c.cols)
		}
	}
	// Quoted commas do not outvote the real delimiter.
	if d := DetectDelim([]byte("a;b;c\n\"x,y,z,w\";2;3\n4;5;6\n")); d != ';' {
		t.Fatalf("quoted commas: %q", d)
	}
	// One column: nothing splits, the comma is the fallback.
	if d := DetectDelim([]byte("name\nann\nbo\n")); d != ',' {
		t.Fatalf("single column: %q", d)
	}
}

func TestReadQuotesBOMRaggedAndNoHeader(t *testing.T) {
	tab, err := Read(strings.NewReader("\ufeffa,b\n1,\"two, too\"\n3\n4,5,6\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tab.Header, []string{"a", "b", "C"}) {
		t.Fatalf("header = %q", tab.Header)
	}
	want := [][]string{{"1", "two, too", ""}, {"3", "", ""}, {"4", "5", "6"}}
	if !reflect.DeepEqual(tab.Rows, want) {
		t.Fatalf("rows = %q", tab.Rows)
	}
	tab, err = Read(strings.NewReader("1|2\n3|4\n"), Options{NoHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tab.Header, []string{"A", "B"}) || len(tab.Rows) != 2 || tab.Delim != '|' {
		t.Fatalf("no header: %q %q %q", tab.Header, tab.Rows, tab.Delim)
	}
	tab, err = Read(strings.NewReader("a,b\n1,2\n"), Options{Delim: ';'})
	if err != nil || len(tab.Header) != 1 || tab.Header[0] != "a,b" {
		t.Fatalf("forced delimiter: %v %q", err, tab.Header)
	}
}

func TestParseDelimAndLetters(t *testing.T) {
	for in, want := range map[string]rune{"": 0, "auto": 0, "tab": '\t', `\t`: '\t', ",": ',', "pipe": '|', "semicolon": ';', ":": ':'} {
		if got, err := ParseDelim(in); err != nil || got != want {
			t.Errorf("ParseDelim(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseDelim("ab"); err == nil {
		t.Error("two characters accepted")
	}
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := ColumnLetter(i); got != want {
			t.Errorf("ColumnLetter(%d) = %s, want %s", i, got, want)
		}
	}
}

func TestCompareCellsSortsNumbersNumericallyAndEmptiesLast(t *testing.T) {
	values := []string{"10", "", "9", "apple", "Banana", "2.5", "-1"}
	slices.SortStableFunc(values, compareCells)
	want := []string{"-1", "2.5", "9", "10", "apple", "Banana", ""}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("sorted = %q, want %q", values, want)
	}
}

func TestSummarizeGuessesTypes(t *testing.T) {
	tab, err := ReadFile("testdata/cities.csv", Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := Summarize(tab)
	if s.Rows != 10 || s.Columns != 7 || s.Delim != "comma" {
		t.Fatalf("counts: %+v", s)
	}
	types := map[string]string{}
	for _, c := range s.Each {
		types[c.Name] = c.Type
	}
	want := map[string]string{"city": "string", "population": "int", "founded": "int", "capital": "bool", "notes": "string"}
	for k, v := range want {
		if types[k] != v {
			t.Errorf("%s: %s, want %s", k, types[k], v)
		}
	}
	notes := s.Each[6]
	if notes.Empty != 1 || notes.Distinct != 9 {
		t.Errorf("notes: %+v", notes)
	}
	pop := s.Each[2]
	if pop.Min != "286000" || pop.Max != "15388000" {
		t.Errorf("population min/max: %s %s", pop.Min, pop.Max)
	}
	if got := Summarize(&Table{Header: []string{"x"}, Rows: [][]string{{"1.5"}, {"2"}}}).Each[0].Type; got != "float" {
		t.Errorf("int+float = %s", got)
	}
}

func TestRowCSVQuotes(t *testing.T) {
	if got := RowCSV([]string{"a", "b, c", `say "hi"`}, ','); got != `a,"b, c","say ""hi"""` {
		t.Fatalf("RowCSV = %s", got)
	}
	if got := RowCSV([]string{"a", "b"}, '\t'); got != "a\tb" {
		t.Fatalf("tab RowCSV = %q", got)
	}
}
