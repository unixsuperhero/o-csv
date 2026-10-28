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
// the file in process and prints JSON (or, with --format, a table), and
// o-csv catalog describes the noun for a host that mounts the bin. Nothing
// here writes a file.

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

// ShowResult is what Show selects: the header, how many rows matched
// before the window, and the rows in it with their 1-based row numbers in
// the file (Lines). table show answers it as rows (showRows).
type ShowResult struct {
	Columns []string
	Total   int
	Offset  int
	Rows    []Record
	Lines   []int
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
	// rows are indexes into t.Rows, so each row keeps its file number.
	rows := make([]int, len(t.Rows))
	for i := range rows {
		rows[i] = i
	}
	if q.Search != "" {
		needle := strings.ToLower(q.Search)
		rows = slices.DeleteFunc(rows, func(i int) bool {
			return !slices.ContainsFunc(t.Rows[i], func(v string) bool { return strings.Contains(strings.ToLower(v), needle) })
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
		rows = slices.DeleteFunc(rows, func(i int) bool { return !strings.Contains(strings.ToLower(t.Rows[i][j]), needle) })
	}
	if q.Sort != "" {
		j, err := index(q.Sort)
		if err != nil {
			return ShowResult{}, err
		}
		slices.SortStableFunc(rows, func(x, y int) int {
			if q.Desc {
				return compareCells(t.Rows[y][j], t.Rows[x][j])
			}
			return compareCells(t.Rows[x][j], t.Rows[y][j])
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
			values[i] = t.Rows[row][j]
		}
		out.Rows = append(out.Rows, Record{Header: header, Values: values})
		out.Lines = append(out.Lines, row+1)
	}
	return out, nil
}

// showRows is table show's answer: one row per selected CSV row, in the
// family's row shape (ochazuke/cli's Row): noun row, id row/N (N its
// 1-based row in the file, whatever the sort), data the row as an object
// keyed by column in header order, cells its values in column order. The
// cells over the row noun's columns (rowColumns) are what --format table,
// markdown and toon print.
func showRows(res ShowResult) []ocli.Row {
	out := make([]ocli.Row, len(res.Rows))
	for i, rec := range res.Rows {
		out[i] = ocli.Row{Noun: "row", ID: fmt.Sprintf("row/%d", res.Lines[i]), Data: rec, Cells: rec.Values}
	}
	return out
}

// rowColumns are the columns of the row noun for a table show: one per
// shown column, numbers (the type guess over the whole file) right-aligned.
func rowColumns(t *Table, shown []string) []ochazuke.Column {
	types := map[string]string{}
	for _, c := range Columns(t) {
		types[c.Name] = c.Type
	}
	cols := make([]ochazuke.Column, len(shown))
	for i, name := range shown {
		cols[i] = ochazuke.Column{Title: name}
		if types[name] == TypeInt || types[name] == TypeFloat {
			cols[i].Align = ochazuke.AlignRight
		}
	}
	return cols
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
	if err := declareSettings(r); err != nil {
		panic(err) // the declarations are constants; a test covers them
	}
	read := func(args ochazuke.Arguments, delim string) (*Table, error) {
		path := ochazuke.StringArg(args, "path", "")
		if path == "" {
			return nil, errors.New("a file is required (- reads stdin)")
		}
		d, err := ParseDelim(ochazuke.StringArg(args, "delim", delim))
		if err != nil {
			return nil, err
		}
		noHeader, _ := args["no_header"].(bool)
		return ReadFile(path, Options{Delim: d, NoHeader: noHeader})
	}
	// Each verb reads the file as pending work, because reading a file is
	// I/O and application I/O never runs on the loop. The shell waits for
	// the job and prints its value; a failure also prints its
	// "job #N table.VERB failed" line on stderr.
	pending := func(c ochazuke.Ctx, fn func(*Table) (any, error)) ochazuke.Result {
		// The setting is read here, on the loop; the work runs off it.
		args, delim := c.Args, r.Settings().String(settingDelimiter)
		return ochazuke.Pending(func(context.Context) (any, error) {
			t, err := read(args, delim)
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
			// The work reads the file off the loop; Then runs on the loop
			// once it lands, before the shell prints. It gives the row noun
			// this file's columns (they are known only once it is read)
			// and says on the status line when the window cut rows.
			var shown []ochazuke.Column
			var res ShowResult
			return pending(c, func(t *Table) (any, error) {
				var err error
				if res, err = Show(t, q); err != nil {
					return nil, err
				}
				shown = rowColumns(t, res.Columns)
				return showRows(res), nil
			}).Then(func(c ochazuke.Ctx, _ any, err error) {
				if err != nil {
					return
				}
				r.Define("row").Columns(shown...)
				if n := len(res.Rows); n < res.Total {
					c.Status(fmt.Sprintf("rows %d–%d of %d", res.Offset+1, res.Offset+n, res.Total))
				}
			})
		}, "the rows that match, as rows {noun: row, id: row/N (N the row in the file), data: {COLUMN: value, ...} in header order, cells}; filter, sort, pick columns and page with the options; --format table|markdown|toon prints them as a table").Rows("row").Fetch().Quiet().Positional("path").
		Verb("columns", func(c ochazuke.Ctx) ochazuke.Result {
			return pending(c, func(t *Table) (any, error) { return Columns(t), nil })
		}, "the header: [{index, name, type}], type guessed from every value (empty, bool, int, float, date, string)").Fetch().Quiet().Positional("path").
		Verb("stats", func(c ochazuke.Ctx) ochazuke.Result {
			return pending(c, func(t *Table) (any, error) { return Summarize(t), nil })
		}, "row and column counts, the delimiter, and per column: type guess, empty and distinct counts, min, max, widest value").Quiet().Positional("path").Fetch()
	file := map[string]ochazuke.Parameter{
		"path":      {Type: "string", Required: true, Description: "the CSV file; - reads stdin"},
		"delim":     {Type: "string", Description: "field delimiter: one character, or auto, comma, tab, semicolon, pipe (default: the delimiter setting, auto)"},
		"no_header": {Type: "boolean", Description: "the first line is data; columns are named A, B, C, ..."},
	}
	params := func(key string, extra map[string]ochazuke.Parameter) {
		op := r.Operations[r.CanonicalKey(key)]
		op.Parameters = map[string]ochazuke.Parameter{}
		for k, v := range file {
			op.Parameters[k] = v
		}
		for k, v := range extra {
			op.Parameters[k] = v
		}
		op.Result = map[string]string{"table.show": "row[]", "table.columns": "column[]", "table.stats": "stats"}[key]
		r.Operations[r.CanonicalKey(key)] = op
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
