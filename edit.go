package main

import (
	"fmt"
	"sort"
	"strings"

	"ofam.dev/ochazuke"
	"ofam.dev/ochazuke/grid"
)

// Edit mode helpers, from ochazuke's examples/csv: named transformations
// and the shaped yanks and pastes of visual selections.

// Transformations are data behind one verb. A text transformation sees the
// value; a sheet transformation sees a read view of the sheet and the cells
// being transformed. Both produce the same cell patch.

var textTransformations = map[string]func(string) string{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
	"title": func(v string) string {
		words := strings.Fields(strings.ToLower(v))
		for i, w := range words {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
		return strings.Join(words, " ")
	},
	"slug": func(v string) string {
		return strings.Trim(strings.Join(strings.FieldsFunc(strings.ToLower(v), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		}), "-"), "-")
	},
}

var sheetTransformations = map[string]func(a *App, at grid.Ref, selected []grid.Ref) string{
	// fillDown gives every selected cell the value of the topmost selected
	// cell in its column, as a spreadsheet's fill-down does.
	"fillDown": func(a *App, at grid.Ref, selected []grid.Ref) string {
		top := at
		for _, ref := range selected {
			if ref.Col == at.Col && ref.Row < top.Row {
				top = ref
			}
		}
		return a.Cell(top)
	},
}

// TransformationNames lists every transformation, text ones first.
func TransformationNames() []string {
	var text, sheet []string
	for name := range textTransformations {
		text = append(text, name)
	}
	for name := range sheetTransformations {
		sheet = append(sheet, name)
	}
	sort.Strings(text)
	sort.Strings(sheet)
	return append(text, sheet...)
}

// transformationLabels describe each transformation in the pick list.
var transformationLabels = map[string]string{
	"upper":    "UPPER CASE",
	"lower":    "lower case",
	"trim":     "trim surrounding spaces",
	"title":    "Title Case",
	"slug":     "slug-case",
	"fillDown": "fill down from the top selected cell",
}

// TransformationChoices is transformations.list: every name with a label,
// in TransformationNames order, the shape a parameter Source returns.
func TransformationChoices() []ochazuke.Choice {
	names := TransformationNames()
	out := make([]ochazuke.Choice, len(names))
	for i, name := range names {
		out[i] = ochazuke.Choice{ID: name, Label: transformationLabels[name]}
	}
	return out
}

// Transform builds the patch that applies a named transformation to cells.
func (a *App) Transform(name string, cells []grid.Ref) (Patch, error) {
	text, isText := textTransformations[name]
	sheet, isSheet := sheetTransformations[name]
	if !isText && !isSheet {
		return Patch{}, fmt.Errorf("unknown transformation %q; transformations.list names them", name)
	}
	var p Patch
	for _, ref := range cells {
		before := a.Cell(ref)
		after := before
		if isText {
			after = text(before)
		} else {
			after = sheet(a, ref, cells)
		}
		if after != before {
			p.Cells = append(p.Cells, CellChange{At: ref, Before: before, After: after})
		}
	}
	return p, nil
}

// Yanks: the shared registers keep the shape so a row yank pastes rows and
// a column yank pastes columns. Text is rows of tab-separated cells, one
// row per line; a column yank's first line is the header names.

func (a *App) yankOf(cells []grid.Ref, shape string) ochazuke.Yank {
	top, bottom, left, right := grid.Bounds(cells)
	set := map[grid.Ref]bool{}
	for _, ref := range cells {
		set[ref] = true
	}
	var lines []string
	if shape == grid.ShapeColumn {
		var names []string
		for c := left; c <= right; c++ {
			if set[grid.Ref{Row: top, Col: c}] {
				names = append(names, a.Header[c])
			}
		}
		lines = append(lines, strings.Join(names, "\t"))
	}
	for r := top; r <= bottom; r++ {
		var row []string
		for c := left; c <= right; c++ {
			if shape == grid.ShapeColumn && !set[grid.Ref{Row: top, Col: c}] {
				continue
			}
			if set[grid.Ref{Row: r, Col: c}] || shape == grid.ShapeColumn {
				row = append(row, a.Cell(grid.Ref{Row: r, Col: c}))
			} else {
				row = append(row, "")
			}
		}
		if shape == grid.ShapeRow && !set[grid.Ref{Row: r, Col: left}] {
			continue
		}
		lines = append(lines, strings.Join(row, "\t"))
	}
	switch shape {
	case grid.ShapeRow:
		return ochazuke.Yank{Text: strings.Join(lines, "\n") + "\n", Shape: "line"}
	case grid.ShapeColumn:
		return ochazuke.Yank{Text: strings.Join(lines, "\n"), Shape: "column", Width: right - left + 1}
	}
	return ochazuke.Yank{Text: strings.Join(lines, "\n"), Shape: "block", Width: right - left + 1}
}

// pastePatch turns a yank into the patch that pastes it at the cursor: rows
// below the cursor row, columns after the cursor column, a block filling a
// rectangle from the cursor (clipped to the sheet). Any other text pastes as
// a block of tab-separated lines.
func (a *App) pastePatch(y ochazuke.Yank, at grid.Ref) Patch {
	if y.Text == "" {
		return Patch{}
	}
	lines := strings.Split(strings.TrimSuffix(y.Text, "\n"), "\n")
	var p Patch
	switch y.Shape {
	case "line":
		for i, line := range lines {
			p.Rows = append(p.Rows, RowChange{Index: at.Row + 1 + i, After: a.pad(strings.Split(line, "\t"))})
		}
	case "column":
		names := strings.Split(lines[0], "\t")
		for j, name := range names {
			col := Column{Name: name, Values: make([]string, len(a.Rows))}
			for r := range a.Rows {
				if r+1 < len(lines) {
					if cells := strings.Split(lines[r+1], "\t"); j < len(cells) {
						col.Values[r] = cells[j]
					}
				}
			}
			p.Columns = append(p.Columns, ColumnChange{Index: at.Col + 1 + j, After: &col})
		}
	default:
		for i, line := range lines {
			for j, value := range strings.Split(line, "\t") {
				ref := grid.Ref{Row: at.Row + i, Col: at.Col + j}
				if ref.Row >= len(a.Rows) || ref.Col >= len(a.Header) {
					continue
				}
				if before := a.Cell(ref); before != value {
					p.Cells = append(p.Cells, CellChange{At: ref, Before: before, After: value})
				}
			}
		}
	}
	return p
}
