// The sheet: the state o-csv owns. The runtime holds only interaction state
// (focus, modes, modals, the registers); the grid component holds the
// cursor and the visible rows; everything about the file lives here.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/unixsuperhero/ochazuke"
	"github.com/unixsuperhero/ochazuke/grid"
)

// App is the viewer's state. Domain data never lives on an instance:
// instances carry only ids (r12, c2, r12:c2).
type App struct {
	Path   string
	Opts   Options
	Delim  rune
	Header []string
	Rows   [][]string
	// loaded is the file order of the rows while nothing has been edited,
	// so a third sort on one column restores it. Any edit drops it.
	loaded [][]string
	// Edit allows edits and saving. Off, nothing the app does writes a file.
	Edit  bool
	Dirty bool
	Undo  []Patch
	Redo  []Patch

	Filter FilterState
	Sort   SortState
	Search string
	Hidden map[int]bool
	Freeze bool
	// ShowDetail draws the cursor row's detail beside or below the table.
	ShowDetail bool
	Loading    string // the path being read off the loop, "" when idle
	LoadErr    error

	// Clipboard copies text to the system clipboard; nil when there is none.
	Clipboard func(string) error
	// clipNotice is the footer notice of the last failed clipboard copy.
	clipNotice ochazuke.NoticeID

	widths []int // display width per column, from the content (view.go)
	colCap int   // the column-width setting: the widest a column grows
	view   viewState

	r        *ochazuke.Runtime
	m        *ochazuke.Model
	grid     *grid.Grid
	detail   *ochazuke.Detail
	field    *ochazuke.Input
	filterIn *ochazuke.Input
	ask      *question
}

// FilterState is the row filter the sheet shows through: view state, not an edit.
type FilterState struct {
	Col   int    `json:"col"`
	Query string `json:"query"`
}

// SortState is the column the rows are ordered by; On false is file order.
type SortState struct {
	On   bool `json:"on"`
	Col  int  `json:"col"`
	Desc bool `json:"desc"`
}

// A Patch is the one edit shape: every change to the sheet is a patch, and
// the same patch, inverted, is its undo record. Cells replace values; Rows
// and Columns insert (Before nil), delete (After nil) or replace by index.
type Patch struct {
	Cells   []CellChange   `json:"cells,omitempty"`
	Rows    []RowChange    `json:"rows,omitempty"`
	Columns []ColumnChange `json:"columns,omitempty"`
}

type CellChange struct {
	At     grid.Ref `json:"at"`
	Before string   `json:"before"`
	After  string   `json:"after"`
}

type RowChange struct {
	Index  int      `json:"index"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

// Column is a header name with the column's values, one per row.
type Column struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type ColumnChange struct {
	Index  int     `json:"index"`
	Before *Column `json:"before,omitempty"`
	After  *Column `json:"after,omitempty"`
}

// Empty reports a patch that changes nothing.
func (p Patch) Empty() bool { return len(p.Cells) == 0 && len(p.Rows) == 0 && len(p.Columns) == 0 }

// Inverse swaps before and after in every change.
func (p Patch) Inverse() Patch {
	out := Patch{}
	for _, c := range p.Cells {
		out.Cells = append(out.Cells, CellChange{At: c.At, Before: c.After, After: c.Before})
	}
	for _, r := range p.Rows {
		out.Rows = append(out.Rows, RowChange{Index: r.Index, Before: r.After, After: r.Before})
	}
	for _, c := range p.Columns {
		out.Columns = append(out.Columns, ColumnChange{Index: c.Index, Before: c.After, After: c.Before})
	}
	return out
}

// errReadOnly is every edit's answer while editing is off.
var errReadOnly = errors.New("read-only: start with --edit or press ctrl+e to edit")

// Apply records a patch: write it, mark the sheet dirty, push it on the undo
// stack and drop the redo stack. An empty patch is a no-op; any patch is
// refused while editing is off.
func (a *App) Apply(p Patch) error {
	if !a.Edit {
		return errReadOnly
	}
	if p.Empty() {
		return nil
	}
	a.write(p)
	a.Dirty = true
	a.Undo = append(a.Undo, p)
	a.Redo = nil
	return nil
}

// UndoPatch reverts the last patch; RedoPatch replays the last undone one.
func (a *App) UndoPatch() error {
	if !a.Edit {
		return errReadOnly
	}
	if len(a.Undo) == 0 {
		return errors.New("nothing to undo")
	}
	p := a.Undo[len(a.Undo)-1]
	a.Undo = a.Undo[:len(a.Undo)-1]
	a.write(p.Inverse())
	a.Redo = append(a.Redo, p)
	a.Dirty = true
	return nil
}

func (a *App) RedoPatch() error {
	if !a.Edit {
		return errReadOnly
	}
	if len(a.Redo) == 0 {
		return errors.New("nothing to redo")
	}
	p := a.Redo[len(a.Redo)-1]
	a.Redo = a.Redo[:len(a.Redo)-1]
	a.write(p)
	a.Undo = append(a.Undo, p)
	a.Dirty = true
	return nil
}

// write applies a patch to the sheet: column deletions high to low, column
// replacements, column insertions low to high; rows the same way; then
// cells. A patch that adds or removes columns resets the hidden set and
// the sort, whose indexes it moved.
func (a *App) write(p Patch) {
	a.loaded = nil
	cols := append([]ColumnChange(nil), p.Columns...)
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].Index > cols[j].Index })
	for _, c := range cols {
		if c.After == nil && c.Before != nil {
			a.deleteColumn(c.Index)
		}
	}
	for _, c := range cols {
		if c.After != nil && c.Before != nil {
			a.setColumn(c.Index, *c.After)
		}
	}
	for i := len(cols) - 1; i >= 0; i-- {
		if c := cols[i]; c.After != nil && c.Before == nil {
			a.insertColumn(c.Index, *c.After)
		}
	}
	rows := append([]RowChange(nil), p.Rows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Index > rows[j].Index })
	for _, r := range rows {
		if r.After == nil && r.Before != nil && r.Index < len(a.Rows) {
			a.Rows = append(a.Rows[:r.Index], a.Rows[r.Index+1:]...)
		}
	}
	for _, r := range rows {
		if r.After != nil && r.Before != nil && r.Index < len(a.Rows) {
			a.Rows[r.Index] = a.pad(append([]string(nil), r.After...))
		}
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if r := rows[i]; r.After != nil && r.Before == nil {
			at := min(r.Index, len(a.Rows))
			a.Rows = append(a.Rows[:at], append([][]string{a.pad(append([]string(nil), r.After...))}, a.Rows[at:]...)...)
		}
	}
	for _, c := range p.Cells {
		if c.At.Row >= 0 && c.At.Row < len(a.Rows) && c.At.Col >= 0 && c.At.Col < len(a.Header) {
			a.Rows[c.At.Row][c.At.Col] = c.After
		}
	}
	if len(p.Columns) > 0 {
		a.Hidden, a.Sort = map[int]bool{}, SortState{}
		a.measure()
	} else {
		for _, c := range p.Cells {
			a.widen(c.At.Col, c.After)
		}
		for _, r := range p.Rows {
			for j, v := range r.After {
				a.widen(j, v)
			}
		}
	}
	a.grid.SetData(a.Header, a.Rows)
}

func (a *App) deleteColumn(i int) {
	if i < 0 || i >= len(a.Header) {
		return
	}
	a.Header = append(a.Header[:i], a.Header[i+1:]...)
	for r := range a.Rows {
		a.Rows[r] = append(a.Rows[r][:i], a.Rows[r][i+1:]...)
	}
}

func (a *App) setColumn(i int, col Column) {
	if i < 0 || i >= len(a.Header) {
		return
	}
	a.Header[i] = col.Name
	for r := range a.Rows {
		if r < len(col.Values) {
			a.Rows[r][i] = col.Values[r]
		}
	}
}

func (a *App) insertColumn(i int, col Column) {
	i = min(max(i, 0), len(a.Header))
	a.Header = append(a.Header[:i], append([]string{col.Name}, a.Header[i:]...)...)
	for r := range a.Rows {
		value := ""
		if r < len(col.Values) {
			value = col.Values[r]
		}
		a.Rows[r] = append(a.Rows[r][:i], append([]string{value}, a.Rows[r][i:]...)...)
	}
}

// pad fits a row to the header width.
func (a *App) pad(row []string) []string { return padTo(row, len(a.Header)) }

// column reads a column as a value.
func (a *App) column(i int) Column {
	col := Column{Name: a.Header[i], Values: make([]string, len(a.Rows))}
	for r, row := range a.Rows {
		col.Values[r] = row[i]
	}
	return col
}

// Cell reads one value, "" outside the sheet.
func (a *App) Cell(ref grid.Ref) string {
	if ref.Row < 0 || ref.Row >= len(a.Rows) || ref.Col < 0 || ref.Col >= len(a.Header) {
		return ""
	}
	return a.Rows[ref.Row][ref.Col]
}

// setTable replaces the sheet with a parsed file and clears history and
// view state.
func (a *App) setTable(path string, t *Table) {
	a.Path, a.Delim = path, t.Delim
	a.Header, a.Rows = t.Header, t.Rows
	a.loaded = slices.Clone(t.Rows)
	a.Dirty, a.Undo, a.Redo = false, nil, nil
	a.Filter, a.Sort, a.Search, a.Hidden = FilterState{}, SortState{}, "", map[int]bool{}
	a.Loading, a.LoadErr = "", nil
	a.view = viewState{}
	a.measure()
	a.syncFilterField()
	a.grid.SetFilter(nil)
	a.grid.SetData(a.Header, a.Rows)
	if a.r.Page == pageID && a.r.Focus == gridID {
		_ = a.grid.Focus()
	}
}

// Load reads a file on the calling goroutine (tests, --smoke, the CLI). The
// TUI reads through sheet.load, off the loop.
func (a *App) Load(path string) error {
	t, err := ReadFile(path, a.Opts)
	if err != nil {
		return err
	}
	a.setTable(path, t)
	return nil
}

// Save writes the sheet to path and makes it the sheet's path. It refuses
// while editing is off, so a read-only session never writes a file.
func (a *App) Save(path string) error {
	if !a.Edit {
		return errReadOnly
	}
	if path == "" || path == "-" {
		return errors.New("the sheet has no file; use sheet.saveAs")
	}
	t := &Table{Header: a.Header, Rows: a.Rows, Delim: a.Delim}
	if err := writeAtomic(path, func(w io.Writer) error { return Write(w, t, a.Opts.NoHeader) }); err != nil {
		return err
	}
	a.Path, a.Dirty = path, false
	return nil
}

// saveTo is Save with a status line saying what was written. A prompt's
// commit runs it too, and a commit has no Ctx of the verb that asked, so
// it posts through the runtime's UI (what ctx.Status does underneath).
func (a *App) saveTo(path string) error {
	if err := a.Save(path); err != nil {
		return err
	}
	a.r.UI().Status(fmt.Sprintf("wrote %s rows to %s", count(len(a.Rows)), a.describe()))
	return nil
}

// writeAtomic replaces path with what write produces without ever leaving
// it half written: the bytes go to a temporary file beside it (same
// directory, so the rename stays on one file system), which gets the
// original's mode (0644 for a new file), is synced and closed, then
// renamed over path. On any error the temporary file is removed and path
// is untouched.
func writeAtomic(path string, write func(io.Writer) error) (err error) {
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if err = write(f); err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sortBy orders the rows by column j. The first sort on a column is
// ascending, the next descending; a third returns to file order while the
// sheet is unedited. Read-only, the rows are reordered as a view; while
// editing, the order is an undoable patch.
func (a *App) sortBy(j int) error {
	if j < 0 || j >= len(a.Header) {
		return errors.New("no column to sort by")
	}
	next := SortState{On: true, Col: j}
	if a.Sort.On && a.Sort.Col == j {
		next.Desc = !a.Sort.Desc
		if a.Sort.Desc && a.loaded != nil && !a.Edit {
			next = SortState{Col: j}
		}
	}
	order := make([][]string, len(a.Rows))
	copy(order, a.Rows)
	if next.On {
		slices.SortStableFunc(order, func(x, y []string) int {
			if next.Desc {
				return compareCells(y[j], x[j])
			}
			return compareCells(x[j], y[j])
		})
	} else {
		copy(order, a.loaded)
	}
	if !a.Edit {
		a.Rows = order
		a.Sort = next
		a.grid.SetData(a.Header, a.Rows)
		return nil
	}
	var p Patch
	for i := range order {
		if !slices.Equal(order[i], a.Rows[i]) {
			p.Rows = append(p.Rows, RowChange{Index: i, Before: slices.Clone(a.Rows[i]), After: slices.Clone(order[i])})
		}
	}
	if err := a.Apply(p); err != nil {
		return err
	}
	a.Sort = next
	return nil
}

// setEdit turns editing on or off. Turning it off with unsaved edits is
// refused, so the edits are saved or undone first.
func (a *App) setEdit(on bool) error {
	if !on && a.Dirty {
		return errors.New("unsaved edits: save (ctrl+s) or undo them before leaving edit mode")
	}
	a.Edit = on
	return nil
}

// columnName is the header of j, "" outside the sheet.
func (a *App) columnName(j int) string {
	if j < 0 || j >= len(a.Header) {
		return ""
	}
	return a.Header[j]
}

// state is the value a sheet verb returns.
func (a *App) state() map[string]any {
	return map[string]any{"path": a.Path, "edit": a.Edit, "dirty": a.Dirty, "rows": len(a.Rows), "columns": len(a.Header), "visible": a.grid.VisibleCount(), "undo": len(a.Undo), "redo": len(a.Redo), "cursor": a.grid.Cursor(), "filter": a.Filter, "sort": a.Sort, "search": a.Search, "hidden": a.hiddenList(), "freeze": a.Freeze, "detail": a.ShowDetail, "delimiter": DelimName(a.Delim)}
}

func (a *App) hiddenList() []int {
	out := []int{}
	for j := range a.Hidden {
		out = append(out, j)
	}
	sort.Ints(out)
	return out
}

// describe is the sheet's name for the status line.
func (a *App) describe() string {
	switch {
	case a.Path == "-":
		return "(stdin)"
	case a.Path == "":
		return "(no file)"
	}
	if len(a.Path) > 40 {
		return filepath.Base(a.Path)
	}
	return a.Path
}
