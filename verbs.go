package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/unixsuperhero/ochazuke"
	"github.com/unixsuperhero/ochazuke/grid"
)

// Guards: edit holds while editing is on; edit-grid also needs the focus in
// the grid. Every verb that changes the sheet is guarded, so its keys do not
// answer (and the palette does not offer it) read-only, and Apply refuses
// it again underneath.
const (
	guardEdit     = "edit"
	guardEditGrid = "edit-grid"
)

func inGrid(r *ochazuke.Runtime, focus string) bool {
	return slices.Contains(r.Ancestry(focus), gridID)
}

// registerVerbs declares the nouns and their verbs. Keys are bound in
// bindings.go; a verb without a key is reached through the palette (:).
func (a *App) registerVerbs() {
	r := a.r
	r.DefineGuard(guardEdit, func(*ochazuke.Runtime, ochazuke.KeyEvent) bool { return a.Edit })
	r.DefineGuard(guardEditGrid, func(r *ochazuke.Runtime, ev ochazuke.KeyEvent) bool { return a.Edit && inGrid(r, ev.Focus) })
	verb := func(fn func(c ochazuke.Context, args ochazuke.Arguments) (any, error)) ochazuke.Handler {
		return func(_ *ochazuke.Runtime, c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			return fn(c, args)
		}
	}
	params := func(key string, p map[string]ochazuke.Parameter) {
		op := r.Operations[key]
		op.Parameters = p
		r.Operations[key] = op
	}
	done := func(err error) (any, error) {
		if err != nil {
			return nil, err
		}
		return a.state(), nil
	}
	path := map[string]ochazuke.Parameter{"path": {Type: "string", Description: "Omit to be asked in a prompt (ctrl+o there opens the picker)"}}

	// Viewing.
	r.Define("sheet").
		Verb("load", func(c ochazuke.Ctx) ochazuke.Result {
			p := ochazuke.StringArg(c.Args, "path", a.Path)
			if p == "" {
				return ochazuke.Failed(errors.New("no file to load"))
			}
			if a.Dirty {
				return ochazuke.Failed(errors.New("unsaved edits: save or undo them first"))
			}
			opts := a.Opts
			a.Loading, a.LoadErr = p, nil
			return ochazuke.Pending(func(context.Context) (any, error) { return ReadFile(p, opts) }).
				Replacing("load").
				Then(func(_ ochazuke.Ctx, v any, err error) {
					if err != nil {
						a.Loading, a.LoadErr = "", err
						return
					}
					a.setTable(p, v.(*Table))
				})
		}, "Read a CSV file off the loop and show it (path defaults to the current file)").
		Verb("open", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			load := func(p string) error {
				_, err := r.Execute(ochazuke.Request{Noun: "sheet", Verb: "load", Target: pageID, Args: ochazuke.Arguments{"path": p}})
				return err
			}
			if p := ochazuke.StringArg(args, "path", ""); p != "" {
				return nil, load(p)
			}
			return nil, a.prompt("Open file", a.Path, "path", load)
		}), "Open another CSV file in place of this one").
		Verb("move", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			return done(a.move(ochazuke.StringArg(args, "motion", ""), ochazuke.IntArg(args, "steps", 1), ochazuke.IntArg(args, "line", 0)))
		}), "Move the cursor: left, right, up, down, row-start, row-end, top, bottom, half-down, half-up, page-down, page-up; hidden columns are skipped").Motion().
		Verb("column", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			if n := ochazuke.StringArg(args, "name", ""); n != "" {
				return done(a.jumpColumn(n))
			}
			if _, ok := args["index"]; ok {
				return done(a.jumpColumn(strconv.Itoa(ochazuke.IntArg(args, "index", 1))))
			}
			return nil, a.prompt("Go to column (name or number)", "", "", a.jumpColumn)
		}), "Jump to a column by 1-based index or by name (exact, then prefix, then substring); with neither, ask").Motion().
		Verb("search", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			if q, ok := args["query"].(string); ok {
				return done(a.search(q))
			}
			return nil, a.prompt("Search", a.Search, "", a.search)
		}), "Find text in any shown cell (case-insensitive) and jump to the next match; an empty query clears it").
		Verb("next", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return done(a.findNext(1)) }), "Jump to the next search match").
		Verb("previous", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return done(a.findNext(-1)) }), "Jump to the previous search match").
		Verb("clear", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) {
			a.Search = ""
			return done(a.filterBy(max(0, a.Filter.Col), ""))
		}), "Clear the search and the filter").
		Verb("unhide", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) {
			a.Hidden = map[int]bool{}
			return a.state(), nil
		}), "Show every hidden column").
		Verb("freeze", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) {
			a.Freeze = !a.Freeze
			return a.state(), nil
		}), "Keep the first shown column at the left while scrolling (toggle)").
		Verb("edit", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			on := !a.Edit
			if v, ok := args["on"].(bool); ok {
				on = v
			}
			return done(a.setEdit(on))
		}), "Turn editing on or off (toggle); off, nothing writes the file").
		Verb("state", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return a.state(), nil }), "The sheet's path, counts, cursor and view state")
	params("sheet.load", path)
	params("sheet.open", path)
	params("sheet.move", map[string]ochazuke.Parameter{
		"motion": {Type: "string", Required: true, Enum: []string{"left", "right", "up", "down", "row-start", "row-end", "top", "bottom", "half-down", "half-up", "page-down", "page-up"}},
		"steps":  {Type: "integer", Description: "Repeat count (default 1)"},
		"line":   {Type: "integer", Description: "1-based row for top and bottom"},
	})
	params("sheet.column", map[string]ochazuke.Parameter{
		"index": {Type: "integer", Description: "1-based column number"},
		"name":  {Type: "string", Description: "Column name, prefix or substring"},
	})
	params("sheet.search", map[string]ochazuke.Parameter{"query": {Type: "string", Description: "Omit to be asked in a prompt"}})
	params("sheet.edit", map[string]ochazuke.Parameter{"on": {Type: "boolean", Description: "Omit to toggle"}})

	colOf := func(c ochazuke.Context) (int, error) {
		at := a.grid.At(c)
		if at.Col < 0 {
			return 0, errors.New("no column under the cursor")
		}
		return at.Col, nil
	}
	rowOf := func(c ochazuke.Context) (int, error) {
		at := a.grid.At(c)
		if at.Row < 0 {
			return 0, errors.New("no row under the cursor")
		}
		return at.Row, nil
	}
	r.Define("column").
		Verb("sort", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			j, err := colOf(c)
			if err != nil {
				return nil, err
			}
			return done(a.sortBy(j))
		}), "Sort the rows by this column: ascending, then descending, then (unedited) file order").When(grid.GuardInGrid).
		Verb("hide", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			j, err := colOf(c)
			if err != nil {
				return nil, err
			}
			return done(a.hide(j))
		}), "Hide this column (sheet.unhide shows them all again)").When(grid.GuardInGrid).
		Verb("filter", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			j, err := colOf(c)
			if err != nil {
				return nil, err
			}
			if q, ok := args["query"].(string); ok {
				return done(a.filterBy(j, q))
			}
			return nil, a.focusFilter()
		}), "Show only rows whose cell in this column contains the query (omit it to type one in the filter field); an empty query shows every row").When(grid.GuardInGrid)
	params("column.filter", map[string]ochazuke.Parameter{"query": {Type: "string", Description: "Omit to type it in the filter field"}})

	r.Define("row").
		Verb("detail", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			a.toggleDetail()
			return a.state(), nil
		}), "Show or hide the row detail: every column of the cursor's row, beside the table (below it when narrow); J and K scroll it").When(grid.GuardInGrid).
		Verb("yank", func(c ochazuke.Ctx) ochazuke.Result {
			i, err := rowOf(c.Context)
			if err != nil {
				return ochazuke.Failed(err)
			}
			return a.copyText(c, "row", RowCSV(a.Rows[i], a.Delim), ochazuke.StringArg(c.Args, "register", ""))
		}, "Copy the row as one CSV line into the registers and the system clipboard").When(grid.GuardInGrid)

	register := map[string]ochazuke.Parameter{"register": {Type: "string", Description: "Register name; the active register, else the unnamed one"}}
	params("row.yank", register)
	r.Define("cell").
		Verb("yank", func(c ochazuke.Ctx) ochazuke.Result {
			at := a.grid.At(c.Context)
			if at.Row < 0 {
				return ochazuke.Failed(errors.New("no cell to copy"))
			}
			return a.copyText(c, "cell", a.Cell(at), ochazuke.StringArg(c.Args, "register", ""))
		}, "Copy the cell's value into the registers and the system clipboard")
	params("cell.yank", register)

	a.registerEditVerbs(verb, params, done)
}

// move runs a grid motion, then steps off hidden columns in the motion's
// direction (or back, at the edge).
func (a *App) move(motion string, steps, line int) error {
	steps = max(1, steps)
	half := max(1, a.view.height/2)
	switch motion {
	case "half-down", "half-up", "page-down", "page-up":
		n := half
		if strings.HasPrefix(motion, "page") {
			n = max(1, a.view.height-1)
		}
		motion, steps = strings.TrimPrefix(strings.TrimPrefix(motion, "half-"), "page-"), steps*n
	}
	if err := a.grid.Move(motion, steps, line); err != nil {
		return err
	}
	dir := 1
	if motion == "left" || motion == "row-end" {
		dir = -1
	}
	return a.skipHidden(dir)
}

// skipHidden moves the cursor off a hidden column, dir first.
func (a *App) skipHidden(dir int) error {
	cur := a.grid.Cursor()
	if cur.Row < 0 || !a.Hidden[cur.Col] {
		return nil
	}
	for _, d := range []int{dir, -dir} {
		for j := cur.Col + d; j >= 0 && j < len(a.Header); j += d {
			if !a.Hidden[j] {
				return a.grid.Goto(grid.Ref{Row: cur.Row, Col: j})
			}
		}
	}
	return nil
}

// hide hides column j and moves the cursor to the next shown column. The
// last shown column cannot be hidden.
func (a *App) hide(j int) error {
	if len(a.showable()) <= 1 {
		return errors.New("the last shown column cannot be hidden")
	}
	a.Hidden[j] = true
	return a.skipHidden(1)
}

// jumpColumn moves the cursor to a column named by its 1-based number or
// its name: an exact match (case-insensitive), else a prefix, else a
// substring. A hidden column is shown again.
func (a *App) jumpColumn(spec string) error {
	spec = strings.TrimSpace(spec)
	j := -1
	if n, err := strconv.Atoi(spec); err == nil {
		if n < 1 || n > len(a.Header) {
			return fmt.Errorf("no column %d (the sheet has %d)", n, len(a.Header))
		}
		j = n - 1
	}
	low := strings.ToLower(spec)
	for _, match := range []func(string) bool{
		func(h string) bool { return h == low },
		func(h string) bool { return strings.HasPrefix(h, low) },
		func(h string) bool { return strings.Contains(h, low) },
	} {
		for k := 0; j < 0 && k < len(a.Header); k++ {
			if match(strings.ToLower(a.Header[k])) {
				j = k
			}
		}
	}
	if j < 0 {
		return fmt.Errorf("no column matches %q", spec)
	}
	cur := a.grid.Cursor()
	if cur.Row < 0 {
		return errors.New("no rows")
	}
	delete(a.Hidden, j)
	return a.grid.Goto(grid.Ref{Row: cur.Row, Col: j})
}

// search sets the query and jumps to the first match at or after the cursor.
func (a *App) search(q string) error {
	a.Search = q
	if q == "" {
		return nil
	}
	return a.find(1, true)
}

// findNext jumps to the next (dir 1) or previous (dir -1) match.
func (a *App) findNext(dir int) error {
	if a.Search == "" {
		return errors.New("no search: press / to search")
	}
	return a.find(dir, false)
}

// find walks the visible rows in reading order from the cursor (including
// it when here is set), wrapping once, and moves to the first shown cell
// containing the query.
func (a *App) find(dir int, here bool) error {
	visible := a.grid.Visible()
	cols := a.showable()
	if len(visible) == 0 || len(cols) == 0 {
		return errors.New("nothing to search")
	}
	vrow, col := a.grid.Position()
	ci := max(0, slices.Index(cols, col))
	needle := strings.ToLower(a.Search)
	total := len(visible) * len(cols)
	start := vrow*len(cols) + ci
	for step := 0; step <= total; step++ {
		if step == 0 && !here {
			continue
		}
		k := ((start+dir*step)%total + total) % total
		v, c := k/len(cols), cols[k%len(cols)]
		row := visible[v]
		if strings.Contains(strings.ToLower(a.Rows[row][c]), needle) {
			return a.grid.Goto(grid.Ref{Row: row, Col: c})
		}
	}
	return fmt.Errorf("no match for %q", a.Search)
}

// copyText puts text in the registers now and on the system clipboard off
// the loop, and says so on the status line. A clipboard that fails is a
// notice (it stays until a later copy reaches the clipboard), because the
// copy still landed in the registers.
func (a *App) copyText(c ochazuke.Ctx, what, text, register string) ochazuke.Result {
	y := ochazuke.Yank{Text: text}
	if err := a.r.StoreYank(y, register, false); err != nil {
		return ochazuke.Failed(err)
	}
	copied := fmt.Sprintf("copied the %s (%s characters)", what, count(len([]rune(text))))
	clip := a.Clipboard
	if clip == nil {
		c.Status(copied + " to the registers")
		return ochazuke.Done(y)
	}
	return ochazuke.Pending(func(context.Context) (any, error) { return y, clip(text) }).
		Then(func(c ochazuke.Ctx, _ any, err error) {
			c.ClearNotice(a.clipNotice)
			a.clipNotice = 0
			if err != nil {
				a.clipNotice = c.Notify("clipboard: " + err.Error() + " (copies still reach the registers)")
				return
			}
			c.Status(copied)
		})
}

// registerEditVerbs are the edit mode verbs of ochazuke's CSV example,
// each guarded so it answers only while editing.
func (a *App) registerEditVerbs(verb func(func(ochazuke.Context, ochazuke.Arguments) (any, error)) ochazuke.Handler, params func(string, map[string]ochazuke.Parameter), done func(error) (any, error)) {
	r := a.r
	name := map[string]ochazuke.Parameter{"name": {Type: "string", Description: "Omit to be asked in a prompt"}}
	path := map[string]ochazuke.Parameter{"path": {Type: "string", Description: "Omit to be asked in a prompt (ctrl+o there opens the picker)"}}
	r.Define("sheet").
		Verb("save", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			if a.Path == "" || a.Path == "-" {
				return nil, a.prompt("Save as", "", "path", a.saveTo)
			}
			return done(a.saveTo(a.Path))
		}), "Write the sheet to its file (edit mode)").When(guardEdit).
		Verb("saveAs", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			if p := ochazuke.StringArg(args, "path", ""); p != "" {
				return done(a.saveTo(p))
			}
			return nil, a.prompt("Save as", a.Path, "path", a.saveTo)
		}), "Write the sheet to another file and keep editing it there (edit mode)").When(guardEdit).
		Verb("addColumn", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			add := func(name string) error {
				return a.Apply(Patch{Columns: []ColumnChange{{Index: len(a.Header), After: &Column{Name: name, Values: make([]string, len(a.Rows))}}}})
			}
			if n := ochazuke.StringArg(args, "name", ""); n != "" {
				return done(add(n))
			}
			return nil, a.prompt("New column name", "", "", add)
		}), "Append a column (edit mode)").When(guardEdit).
		Verb("undo", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return done(a.UndoPatch()) }), "Revert the last edit").When(guardEdit).
		Verb("redo", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return done(a.RedoPatch()) }), "Replay the last undone edit").When(guardEdit)
	params("sheet.saveAs", path)
	params("sheet.addColumn", name)

	// cells resolves the cells a verb acts on: the selection when one is
	// active, else the focused cell.
	cells := func(c ochazuke.Context) []grid.Ref {
		if sel := a.grid.Selected(); len(sel) > 0 {
			return sel
		}
		if at := a.grid.At(c); at.Row >= 0 {
			return []grid.Ref{at}
		}
		return nil
	}
	paste := verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
		y, err := r.ReadYank(ochazuke.StringArg(args, "register", ""))
		if err != nil {
			return nil, err
		}
		at := a.grid.At(c)
		if set := a.grid.Selected(); len(set) > 0 {
			at = set[0]
		}
		a.grid.Cancel()
		if at.Row < 0 && y.Shape != "line" && y.Shape != "column" {
			return nil, errors.New("nothing to paste onto")
		}
		return done(a.Apply(a.pastePatch(y, at)))
	})
	clear := verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
		var p Patch
		for _, ref := range cells(c) {
			if before := a.Cell(ref); before != "" {
				p.Cells = append(p.Cells, CellChange{At: ref, Before: before})
			}
		}
		a.grid.Cancel()
		return done(a.Apply(p))
	})
	transform := verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
		p, err := a.Transform(ochazuke.StringArg(args, "name", ""), cells(c))
		if err != nil {
			return nil, err
		}
		a.grid.Cancel()
		return done(a.Apply(p))
	})
	register := map[string]ochazuke.Parameter{"register": {Type: "string", Description: "Register name; the active register, else the unnamed one"}}
	transformParams := map[string]ochazuke.Parameter{"name": {Type: "string", Required: true, Source: "transformations.list", Description: "A transformation; transformations.list names them"}}

	r.Define("cell").
		Verb("edit", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			at := a.grid.At(c)
			if at.Row < 0 {
				return nil, errors.New("no cell to edit")
			}
			return nil, a.prompt("Edit "+a.columnName(at.Col)+" row "+strconv.Itoa(at.Row+1), a.Cell(at), "", func(text string) error {
				if before := a.Cell(at); before != text {
					return a.Apply(Patch{Cells: []CellChange{{At: at, Before: before, After: text}}})
				}
				return nil
			})
		}), "Edit the cell in a prompt; enter commits, esc cancels (edit mode)").When(guardEditGrid).
		Verb("clear", clear, "Empty the cell (edit mode)").When(guardEditGrid).
		Verb("paste", paste, "Paste the register at the cell by its shape: a block fills, rows go below, columns go after (edit mode)").When(guardEditGrid).
		Verb("transform", transform, "Apply a named transformation to the cell, or to the selection when one is active (edit mode)").When(guardEditGrid)
	params("cell.paste", register)
	params("cell.transform", transformParams)

	rowOf := func(c ochazuke.Context) (int, error) {
		at := a.grid.At(c)
		if at.Row < 0 {
			return 0, errors.New("no row under the cursor")
		}
		return at.Row, nil
	}
	r.Define("row").
		Verb("delete", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			i, err := rowOf(c)
			if err != nil {
				return nil, err
			}
			return done(a.Apply(Patch{Rows: []RowChange{{Index: i, Before: slices.Clone(a.Rows[i])}}}))
		}), "Delete the row under the cursor (edit mode)").Confirm("Delete row {{id}}?").When(guardEditGrid).
		Verb("insertBelow", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			i := a.grid.At(c).Row
			if err := a.Apply(Patch{Rows: []RowChange{{Index: i + 1, After: make([]string, len(a.Header))}}}); err != nil {
				return nil, err
			}
			return done(a.grid.Goto(grid.Ref{Row: i + 1, Col: max(0, a.grid.Cursor().Col)}))
		}), "Insert an empty row below the cursor (edit mode)").When(guardEditGrid).
		Verb("duplicate", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			i, err := rowOf(c)
			if err != nil {
				return nil, err
			}
			return done(a.Apply(Patch{Rows: []RowChange{{Index: i + 1, After: slices.Clone(a.Rows[i])}}}))
		}), "Copy the row below itself (edit mode)").When(guardEditGrid)

	colOf := func(c ochazuke.Context) (int, error) {
		at := a.grid.At(c)
		if at.Col < 0 {
			return 0, errors.New("no column under the cursor")
		}
		return at.Col, nil
	}
	r.Define("column").
		Verb("rename", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			j, err := colOf(c)
			if err != nil {
				return nil, err
			}
			rename := func(name string) error {
				before := a.column(j)
				after := before
				after.Name = name
				if name == before.Name {
					return nil
				}
				return a.Apply(Patch{Columns: []ColumnChange{{Index: j, Before: &before, After: &after}}})
			}
			if n := ochazuke.StringArg(args, "name", ""); n != "" {
				return done(rename(n))
			}
			return nil, a.prompt("Rename column "+a.Header[j], a.Header[j], "", rename)
		}), "Rename the column under the cursor (edit mode)").When(guardEditGrid).
		Verb("delete", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			j, err := colOf(c)
			if err != nil {
				return nil, err
			}
			if n := ochazuke.StringArg(args, "name", ""); n != a.Header[j] {
				return nil, fmt.Errorf("column %s is %q, not %q", grid.ColID(j), a.Header[j], n)
			}
			before := a.column(j)
			return done(a.Apply(Patch{Columns: []ColumnChange{{Index: j, Before: &before}}}))
		}), "Delete the column under the cursor (edit mode)").Confirm("Delete column {{id}} ({{name}})?").When(guardEditGrid)
	params("column.rename", name)
	params("column.delete", map[string]ochazuke.Parameter{"name": {Type: "string", Required: true, Description: "The column's header, which the confirm names; D fills it"}})

	r.Define("selection").
		Verb("yank", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			set := cells(c)
			if len(set) == 0 {
				return nil, errors.New("nothing to yank")
			}
			y := a.yankOf(set, a.grid.Shape())
			a.grid.Cancel()
			return y, r.StoreYank(y, ochazuke.StringArg(args, "register", ""), false)
		}), "Yank the selected cells with their shape").When(grid.GuardSelecting).
		Verb("delete", verb(func(c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			set := a.grid.Selected()
			if len(set) == 0 {
				return nil, errors.New("no selection")
			}
			shape := a.grid.Shape()
			y := a.yankOf(set, shape)
			var p Patch
			switch shape {
			case grid.ShapeRow:
				for _, i := range rowsOf(set) {
					p.Rows = append(p.Rows, RowChange{Index: i, Before: slices.Clone(a.Rows[i])})
				}
			case grid.ShapeColumn:
				for _, j := range colsOf(set) {
					before := a.column(j)
					p.Columns = append(p.Columns, ColumnChange{Index: j, Before: &before})
				}
			default:
				for _, ref := range set {
					if before := a.Cell(ref); before != "" {
						p.Cells = append(p.Cells, CellChange{At: ref, Before: before})
					}
				}
			}
			a.grid.Cancel()
			if err := a.Apply(p); err != nil {
				return nil, err
			}
			return a.state(), r.StoreYank(y, ochazuke.StringArg(args, "register", ""), true)
		}), "Delete the selection into the registers: rows and columns are removed, a block is emptied").When(grid.GuardSelecting).
		Verb("paste", paste, "Paste the register over the selection's first cell by its shape").When(grid.GuardSelecting).
		Verb("clear", clear, "Empty the selected cells").When(grid.GuardSelecting).
		Verb("transform", transform, "Apply a named transformation to the selected cells").When(grid.GuardSelecting)
	params("selection.yank", register)
	params("selection.delete", register)
	params("selection.paste", register)
	params("selection.transform", transformParams)

	r.Define("transformations").
		Verb("list", verb(func(ochazuke.Context, ochazuke.Arguments) (any, error) { return TransformationChoices(), nil }), "List the transformations transform accepts: ids with labels, the transform name's pick list")
}

func rowsOf(set []grid.Ref) []int {
	seen := map[int]bool{}
	var out []int
	for _, ref := range set {
		if !seen[ref.Row] {
			seen[ref.Row] = true
			out = append(out, ref.Row)
		}
	}
	sort.Ints(out)
	return out
}

func colsOf(set []grid.Ref) []int {
	seen := map[int]bool{}
	var out []int
	for _, ref := range set {
		if !seen[ref.Col] {
			seen[ref.Col] = true
			out = append(out, ref.Col)
		}
	}
	sort.Ints(out)
	return out
}
