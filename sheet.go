package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Options are how a file is read: the delimiter (0 detects it) and whether
// the first record is the header.
type Options struct {
	Delim    rune
	NoHeader bool
}

// Table is a parsed file: the header, the rows (each padded to the header's
// width) and the delimiter it was read with. Without a header the columns
// are named A, B, C, ... like a spreadsheet's.
type Table struct {
	Header []string
	Rows   [][]string
	Delim  rune
}

// delimiters are the candidates detection tries, in tie-break order.
var delimiters = []rune{',', '\t', ';', '|'}

// ParseDelim reads a --delim value: one character, or a name (comma, tab,
// semicolon, pipe, \t).
func ParseDelim(s string) (rune, error) {
	switch strings.ToLower(s) {
	case "", "auto":
		return 0, nil
	case "comma", ",":
		return ',', nil
	case "tab", `\t`, "\t":
		return '\t', nil
	case "semicolon", ";":
		return ';', nil
	case "pipe", "|":
		return '|', nil
	}
	if r := []rune(s); len(r) == 1 && r[0] != '"' && r[0] != '\n' && r[0] != '\r' {
		return r[0], nil
	}
	return 0, fmt.Errorf("--delim %q: one character, or comma, tab, semicolon, pipe", s)
}

// DelimName is the delimiter as a word for status lines and JSON.
func DelimName(d rune) string {
	switch d {
	case ',':
		return "comma"
	case '\t':
		return "tab"
	case ';':
		return "semicolon"
	case '|':
		return "pipe"
	}
	return string(d)
}

// DetectDelim picks the delimiter of a sample (the first lines of a file):
// for each candidate it parses the sample and scores it by how many records
// share the most common field count, then by that count. A candidate that
// never splits a line scores nothing; with no winner the comma is used.
func DetectDelim(sample []byte) rune {
	best, bestScore, bestFields := ',', 0, 1
	for _, d := range delimiters {
		r := csv.NewReader(bytes.NewReader(sample))
		r.Comma, r.FieldsPerRecord, r.LazyQuotes = d, -1, true
		counts := map[int]int{}
		for {
			rec, err := r.Read()
			if err != nil {
				// The sample may end mid-record; what parsed so far counts.
				break
			}
			counts[len(rec)]++
		}
		fields, score := 1, 0
		for n, c := range counts {
			if n > 1 && (c > score || c == score && n > fields) {
				fields, score = n, c
			}
		}
		if score > bestScore || score == bestScore && score > 0 && fields > bestFields {
			best, bestScore, bestFields = d, score, fields
		}
	}
	return best
}

// sampleSize is how much of a file detection reads.
const sampleSize = 64 << 10

// Read parses CSV from r. The delimiter is detected from the first 64 KiB
// unless opts names one. Rows shorter than the header are padded; a row
// longer than the header widens it (the new columns are named by letter).
func Read(r io.Reader, opts Options) (*Table, error) {
	br := bufio.NewReaderSize(r, sampleSize)
	delim := opts.Delim
	if delim == 0 {
		sample, err := br.Peek(sampleSize)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
		delim = DetectDelim(trimPartialLine(sample))
	}
	cr := csv.NewReader(br)
	cr.Comma, cr.FieldsPerRecord, cr.LazyQuotes, cr.ReuseRecord = delim, -1, true, false
	t := &Table{Delim: delim}
	width := 0
	first := true
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if first && strings.HasPrefix(rec[0], "\ufeff") {
			rec[0] = strings.TrimPrefix(rec[0], "\ufeff")
		}
		if first && !opts.NoHeader {
			t.Header = rec
			width = len(rec)
			first = false
			continue
		}
		first = false
		width = max(width, len(rec))
		t.Rows = append(t.Rows, rec)
	}
	for len(t.Header) < width {
		t.Header = append(t.Header, ColumnLetter(len(t.Header)))
	}
	for i, row := range t.Rows {
		t.Rows[i] = padTo(row, len(t.Header))
	}
	return t, nil
}

// trimPartialLine drops a sample's last, possibly cut, line when the sample
// has more than one.
func trimPartialLine(sample []byte) []byte {
	if i := bytes.LastIndexByte(sample, '\n'); i > 0 {
		return sample[:i+1]
	}
	return sample
}

// ReadFile parses a file; "-" reads stdin.
func ReadFile(path string, opts Options) (*Table, error) {
	if path == "-" {
		return Read(os.Stdin, opts)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, opts)
}

// Write writes the header (unless noHeader) and rows with the delimiter.
func Write(w io.Writer, t *Table, noHeader bool) error {
	cw := csv.NewWriter(w)
	if t.Delim != 0 {
		cw.Comma = t.Delim
	}
	if !noHeader {
		if err := cw.Write(t.Header); err != nil {
			return err
		}
	}
	if err := cw.WriteAll(t.Rows); err != nil {
		return err
	}
	return cw.Error()
}

// RowCSV is one row as a CSV line (no trailing newline) with the delimiter.
func RowCSV(row []string, delim rune) string {
	var b strings.Builder
	cw := csv.NewWriter(&b)
	if delim != 0 {
		cw.Comma = delim
	}
	_ = cw.Write(row)
	cw.Flush()
	return strings.TrimRight(b.String(), "\r\n")
}

// ColumnLetter names a column like a spreadsheet: A..Z, AA, AB, ...
func ColumnLetter(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}

func padTo(row []string, n int) []string {
	for len(row) < n {
		row = append(row, "")
	}
	return row[:n]
}

// Column types the stats guess, from the most to the least specific.
const (
	TypeEmpty  = "empty"
	TypeBool   = "bool"
	TypeInt    = "int"
	TypeFloat  = "float"
	TypeDate   = "date"
	TypeString = "string"
)

var dateLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "01/02/2006", "2006/01/02"}

// valueType is the type of one nonempty value.
func valueType(v string) string {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no":
		return TypeBool
	}
	if _, err := strconv.ParseInt(v, 10, 64); err == nil {
		return TypeInt
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return TypeFloat
	}
	for _, layout := range dateLayouts {
		if _, err := time.Parse(layout, v); err == nil {
			return TypeDate
		}
	}
	return TypeString
}

// mergeType widens a column's type by one value's type.
func mergeType(col, v string) string {
	switch {
	case col == TypeEmpty || col == v:
		return v
	case col == TypeInt && v == TypeFloat || col == TypeFloat && v == TypeInt:
		return TypeFloat
	}
	return TypeString
}

// ColumnStats describes one column: its guessed type (the narrowest type
// every nonempty value fits), how many values are empty, how many distinct
// values there are, and the shortest and longest value widths. Min and Max
// are numeric for int and float columns, else the smallest and largest
// strings.
type ColumnStats struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Empty    int    `json:"empty"`
	Distinct int    `json:"distinct"`
	Min      string `json:"min,omitempty"`
	Max      string `json:"max,omitempty"`
	Width    int    `json:"width"`
}

// Stats is the table summary table stats prints.
type Stats struct {
	Rows    int           `json:"rows"`
	Columns int           `json:"columns"`
	Delim   string        `json:"delimiter"`
	Each    []ColumnStats `json:"column_stats"`
}

// Summarize computes the stats of a table.
func Summarize(t *Table) Stats {
	s := Stats{Rows: len(t.Rows), Columns: len(t.Header), Delim: DelimName(t.Delim)}
	for j, name := range t.Header {
		c := ColumnStats{Index: j, Name: name, Type: TypeEmpty}
		seen := map[string]bool{}
		var lo, hi string
		var flo, fhi float64
		numeric := true
		for _, row := range t.Rows {
			v := row[j]
			c.Width = max(c.Width, len([]rune(v)))
			if strings.TrimSpace(v) == "" {
				c.Empty++
				continue
			}
			seen[v] = true
			c.Type = mergeType(c.Type, valueType(v))
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && numeric {
				if len(seen) == 1 || f < flo {
					flo = f
				}
				if len(seen) == 1 || f > fhi {
					fhi = f
				}
			} else {
				numeric = false
			}
			if lo == "" || v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		c.Distinct = len(seen)
		if c.Type == TypeInt || c.Type == TypeFloat {
			c.Min, c.Max = strconv.FormatFloat(flo, 'f', -1, 64), strconv.FormatFloat(fhi, 'f', -1, 64)
		} else {
			c.Min, c.Max = lo, hi
		}
		s.Each = append(s.Each, c)
	}
	if s.Each == nil {
		s.Each = []ColumnStats{}
	}
	return s
}

// compareCells orders two cells for sorting: numerically when both parse
// as numbers, empty cells last, else case-insensitively with a byte-order
// tie break.
func compareCells(a, b string) int {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}
