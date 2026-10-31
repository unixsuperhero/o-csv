package main

import (
	"errors"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
)

// The filter field: a persistent single-line input below the grid, the
// sheet's second focus stop. It is a prompt (Readline, insert pinned), so
// } and tab from the grid land on it ready to type; enter filters the rows
// by the cursor column and returns to the grid; tab, shift+tab and esc
// return to the remembered cell without filtering. f on a column focuses
// it; an empty query shows every row. Its text and the row filter stay in step: a filter set by
// column.filter {query} shows in the field.

const filterID = "filter"

// registerFilter adds the field under the sheet, after the grid, so it is
// the next stop in tree order, and binds its keys on the instance.
func (a *App) registerFilter() error {
	r := a.r
	a.filterIn = ochazuke.NewInput(filterID, "", false)
	if err := r.AddInput(a.filterIn, ochazuke.Parent(pageID), ochazuke.Kind("prompt")); err != nil {
		return err
	}
	r.Define("sheet").
		Verb("filter", func(r *ochazuke.Runtime, c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			at := a.grid.Cursor()
			if at.Col < 0 {
				return nil, errors.New("no column to filter by")
			}
			query, ok := args["query"].(string)
			if !ok {
				query = string(a.filterIn.Text)
			}
			if err := a.filterBy(at.Col, query); err != nil {
				return nil, err
			}
			if r.Focus == filterID {
				return a.state(), a.grid.Focus()
			}
			return a.state(), nil
		}, "Filter the rows by the cursor column with the filter field's text (or query); from the field, return to the grid")
	op := r.Operations[r.CanonicalKey("sheet.filter")]
	op.Parameters = map[string]ochazuke.Parameter{"query": {Type: "string", Description: "Omit to use the filter field's text"}}
	r.Operations[r.CanonicalKey("sheet.filter")] = op
	setHistory(r, ochazuke.HistoryAction, "sheet.filter")
	row := func(keys string, b ochazuke.Binding) ochazuke.Binding {
		b.Instance, b.Mode, b.Keys = filterID, "insert", keys
		return b
	}
	return r.Bind(
		row("enter", ochazuke.Do("sheet.filter")),
		row("tab", ochazuke.Do("focus.next")),
		row("shift+tab", ochazuke.Do("focus.previous")),
		row("esc", ochazuke.Do("focus.previous")),
	)
}

// focusFilter moves the focus to the filter field with its cursor at the end.
func (a *App) focusFilter() error {
	a.filterIn.Cursor = len(a.filterIn.Text)
	return a.r.SetFocus(filterID)
}

// syncFilterField shows the active query in the field.
func (a *App) syncFilterField() {
	if string(a.filterIn.Text) == a.Filter.Query {
		return
	}
	_ = a.filterIn.Replace(0, len(a.filterIn.Text), a.Filter.Query)
	a.filterIn.Cursor = len(a.filterIn.Text)
}

// filterLine renders the field with its label, the cursor column's header.
// It reports the cursor only while the field is focused.
func (a *App) filterLine(width int) ochazuke.Block {
	label := "filter: "
	if at := a.grid.Cursor(); at.Col >= 0 && at.Col < len(a.Header) {
		label = "filter " + a.Header[at.Col] + ": "
	}
	label = ansi.Truncate(strings.ReplaceAll(label, "\n", " "), max(1, width/2), "…")
	room := max(1, width-ansi.StringWidth(label))
	if a.r.Focus != filterID {
		return ochazuke.Text(label + ochazuke.InputView(a.filterIn, room, 1))
	}
	field := ochazuke.InputBlock(a.filterIn, room, 1)
	return field.Offset(ansi.StringWidth(label), 0).WithText(label + field.Text)
}

// filterBy shows the rows whose cell in column j contains query (case
// insensitive); "" shows every row. A filter is view state: it works
// read-only and never marks the sheet dirty.
func (a *App) filterBy(j int, query string) error {
	if j < 0 || j >= len(a.Header) {
		return errors.New("no column to filter by")
	}
	a.Filter = FilterState{Col: j, Query: query}
	a.syncFilterField()
	if query == "" {
		a.grid.SetFilter(nil)
		return nil
	}
	needle := strings.ToLower(query)
	a.grid.SetFilter(func(row []string) bool { return j < len(row) && strings.Contains(strings.ToLower(row[j]), needle) })
	return nil
}
