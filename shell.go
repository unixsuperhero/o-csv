package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/unixsuperhero/ochazuke"
	ocli "github.com/unixsuperhero/ochazuke/cli"
)

// The command line is an ochazuke runtime with one noun, table, behind
// ochazuke/cli (mounted in Main): o-csv table VERB FILE [--ARG VALUE] reads
// the file in process and prints JSON, and o-csv catalog describes the
// noun for a host that mounts the bin. Nothing here writes a file.

// Record is one row as a JSON object whose keys keep the header's order.
type Record struct {
	Header []string
	Values []string
}

func (r Record) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, name := range r.Header {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(name)
		v, _ := json.Marshal(r.Values[i])
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// ShowResult is table show's answer: the header, how many rows matched
// before the window, and the rows in it.
type ShowResult struct {
	Columns []string `json:"columns"`
	Total   int      `json:"total"`
	Offset  int      `json:"offset"`
	Rows    []Record `json:"rows"`
}

// ColumnInfo is one element of table columns.
type ColumnInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Type  string `json:"type"`
}

// ShowArgs are table show's options after the file.
type ShowArgs struct {
	Columns []string // names to keep, in this order; empty keeps every column
	Search  string   // keep rows with a cell containing this (case-insensitive)
	Where   string   // COLUMN~TEXT: keep rows whose COLUMN contains TEXT
	Sort    string   // column to sort by
	Desc    bool
	Offset  int
	Limit   int // 0 is every row
}

// Show selects rows of a table: filter, sort, project, then window.
func Show(t *Table, q ShowArgs) (ShowResult, error) {
	index := func(name string) (int, error) {
		for j, h := range t.Header {
			if h == name {
				return j, nil
			}
		}
		for j, h := range t.Header {
			if strings.EqualFold(h, name) {
				return j, nil
			}
		}
		return 0, fmt.Errorf("no column %q; table columns lists them", name)
	}
	rows := t.Rows
	if q.Search != "" {
		needle := strings.ToLower(q.Search)
		rows = slices.DeleteFunc(slices.Clone(rows), func(row []string) bool {
			return !slices.ContainsFunc(row, func(v string) bool { return strings.Contains(strings.ToLower(v), needle) })
		})
	}
	if q.Where != "" {
		name, text, ok := strings.Cut(q.Where, "~")
		if !ok {
			return ShowResult{}, errors.New("where is COLUMN~TEXT")
		}
		j, err := index(name)
		if err != nil {
			return ShowResult{}, err
		}
		needle := strings.ToLower(text)
		rows = slices.DeleteFunc(slices.Clone(rows), func(row []string) bool { return !strings.Contains(strings.ToLower(row[j]), needle) })
	}
	if q.Sort != "" {
		j, err := index(q.Sort)
		if err != nil {
			return ShowResult{}, err
		}
		rows = slices.Clone(rows)
		slices.SortStableFunc(rows, func(x, y []string) int {
			if q.Desc {
				return compareCells(y[j], x[j])
			}
			return compareCells(x[j], y[j])
		})
	}
	keep := make([]int, 0, len(t.Header))
	if len(q.Columns) == 0 {
		for j := range t.Header {
			keep = append(keep, j)
		}
	}
	for _, name := range q.Columns {
		j, err := index(strings.TrimSpace(name))
		if err != nil {
			return ShowResult{}, err
		}
		keep = append(keep, j)
	}
	header := make([]string, len(keep))
	for i, j := range keep {
		header[i] = t.Header[j]
	}
	out := ShowResult{Columns: header, Total: len(rows), Offset: q.Offset, Rows: []Record{}}
	from := min(max(0, q.Offset), len(rows))
	to := len(rows)
	if q.Limit > 0 {
		to = min(to, from+q.Limit)
	}
	for _, row := range rows[from:to] {
		values := make([]string, len(keep))
		for i, j := range keep {
			values[i] = row[j]
		}
		out.Rows = append(out.Rows, Record{Header: header, Values: values})
	}
	return out, nil
}

// Columns is table columns' answer: each column with its guessed type.
func Columns(t *Table) []ColumnInfo {
	stats := Summarize(t)
	out := make([]ColumnInfo, len(stats.Each))
	for i, c := range stats.Each {
		out[i] = ColumnInfo{Index: c.Index, Name: c.Name, Type: c.Type}
	}
	return out
}

// newShell is the NOUN VERB shell over a runtime holding the table noun.
func newShell(stdin io.Reader, stdout, stderr io.Writer) *ocli.Shell {
	r := ochazuke.NewRuntime()
	read := func(args ochazuke.Arguments) (*Table, error) {
		path := ochazuke.StringArg(args, "path", "")
		if path == "" {
			return nil, errors.New("a file is required (- reads stdin)")
		}
		d, err := ParseDelim(ochazuke.StringArg(args, "delim", ""))
		if err != nil {
			return nil, err
		}
		noHeader, _ := args["no_header"].(bool)
		return ReadFile(path, Options{Delim: d, NoHeader: noHeader})
	}
	// Each verb reads the file as pending work: off the loop, and its value
	// reaches the shell as is (a synchronous result is round-tripped
	// through a JSON map, which would sort a row's keys).
	pending := func(c ochazuke.Ctx, fn func(*Table) (any, error)) ochazuke.Result {
		args := c.Args
		return ochazuke.Pending(func(context.Context) (any, error) {
			t, err := read(args)
			if err != nil {
				return nil, err
			}
			return fn(t)
		})
	}
	r.Define("table").
		Verb("show", func(c ochazuke.Ctx) ochazuke.Result {
			desc, _ := c.Args["desc"].(bool)
			var cols []string
			if s := ochazuke.StringArg(c.Args, "columns", ""); s != "" {
				cols = strings.Split(s, ",")
			}
			q := ShowArgs{
				Columns: cols, Search: ochazuke.StringArg(c.Args, "search", ""), Where: ochazuke.StringArg(c.Args, "where", ""),
				Sort: ochazuke.StringArg(c.Args, "sort", ""), Desc: desc,
				Offset: ochazuke.IntArg(c.Args, "offset", 0), Limit: ochazuke.IntArg(c.Args, "limit", 0),
			}
			return pending(c, func(t *Table) (any, error) { return Show(t, q) })
		}, "the rows as JSON objects keyed by column, in header order: {columns, total, offset, rows}; filter, sort, pick columns and page with the options").Quiet().Positional("path").
		Verb("columns", func(c ochazuke.Ctx) ochazuke.Result {
			return pending(c, func(t *Table) (any, error) { return Columns(t), nil })
		}, "the header: [{index, name, type}], type guessed from every value (empty, bool, int, float, date, string)").Quiet().Positional("path").
		Verb("stats", func(c ochazuke.Ctx) ochazuke.Result {
			return pending(c, func(t *Table) (any, error) { return Summarize(t), nil })
		}, "row and column counts, the delimiter, and per column: type guess, empty and distinct counts, min, max, widest value").Quiet().Positional("path")
	file := map[string]ochazuke.Parameter{
		"path":      {Type: "string", Required: true, Description: "the CSV file; - reads stdin"},
		"delim":     {Type: "string", Description: "field delimiter: one character, or comma, tab, semicolon, pipe (default: detect)"},
		"no_header": {Type: "boolean", Description: "the first line is data; columns are named A, B, C, ..."},
	}
	params := func(key string, extra map[string]ochazuke.Parameter) {
		op := r.Operations[key]
		op.Parameters = map[string]ochazuke.Parameter{}
		for k, v := range file {
			op.Parameters[k] = v
		}
		for k, v := range extra {
			op.Parameters[k] = v
		}
		op.Result = map[string]string{"table.show": "{columns, total, offset, rows}", "table.columns": "column[]", "table.stats": "stats"}[key]
		r.Operations[key] = op
	}
	params("table.show", map[string]ochazuke.Parameter{
		"columns": {Type: "string", Description: "comma-separated column names to keep, in that order"},
		"search":  {Type: "string", Description: "keep rows with any cell containing this text (case-insensitive)"},
		"where":   {Type: "string", Description: "COLUMN~TEXT: keep rows whose COLUMN contains TEXT (case-insensitive)"},
		"sort":    {Type: "string", Description: "column to sort by (numbers numerically, empty cells last)"},
		"desc":    {Type: "boolean", Description: "sort descending"},
		"offset":  {Type: "integer", Description: "rows to skip after filtering and sorting"},
		"limit":   {Type: "integer", Description: "most rows to print (default every row)"},
	})
	params("table.columns", nil)
	params("table.stats", nil)
	sh := ocli.New(r, "o-csv", stdin, stdout, stderr)
	sh.Usage = ocli.DefaultUsage("o-csv")
	return sh
}
