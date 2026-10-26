package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/unixsuperhero/ochazuke"
	"github.com/unixsuperhero/ochazuke/grid"
)

// The table view. The grid component owns the cursor and the visible rows;
// this file draws them, because a viewer needs what the grid's own View
// does not do: hidden columns, a frozen first column, widths measured once
// instead of on every frame, a sort marker and search highlights. Only the
// rows and columns on screen are drawn, so a 100k-row file costs a screen.

// widthSample is how many rows measure a column at load; edits widen it.
const widthSample = 2000

// Cell widths are at least minCellWidth and at most the column-width
// setting (App.colCap); longer cells are cut with "…".
const minCellWidth = 3

// viewState is the scroll position of the last frame and what it showed.
type viewState struct {
	offRow   int   // first visible row index drawn
	offCol   int   // first scrolling column, as an index into the shown columns
	height   int   // body rows of the last frame
	shown    []int // the columns the last frame drew, frozen first
	scrolled int   // how many scrolling columns there are
}

// measure computes every column's width from the header and the first rows.
func (a *App) measure() {
	a.widths = make([]int, len(a.Header))
	for j, name := range a.Header {
		a.widths[j] = ansi.StringWidth(name) + 2 // room for the sort marker
	}
	for i := 0; i < len(a.Rows) && i < widthSample; i++ {
		for j, v := range a.Rows[i] {
			a.widen(j, v)
		}
	}
	for j := range a.widths {
		a.widths[j] = min(max(a.widths[j], minCellWidth), a.colCap)
	}
}

// widen grows column j to fit v (up to the cap).
func (a *App) widen(j int, v string) {
	if j < 0 || j >= len(a.widths) {
		return
	}
	a.widths[j] = min(max(a.widths[j], ansi.StringWidth(v)), a.colCap)
}

// showable lists the columns that are not hidden, in order.
func (a *App) showable() []int {
	out := make([]int, 0, len(a.Header))
	for j := range a.Header {
		if !a.Hidden[j] {
			out = append(out, j)
		}
	}
	return out
}

// styles of the table.
var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Reverse(true)
	matchStyle  = lipgloss.NewStyle().Underline(true)
	selectStyle = lipgloss.NewStyle().Underline(true).Bold(true)
	gutterStyle = lipgloss.NewStyle().Faint(true)
)

// render is the sheet page: the status line, the table and the filter field.
func (a *App) render(_ *ochazuke.Runtime, width, height int) ochazuke.Block {
	s := ochazuke.NewStyles(*a.m.Appearance)
	table := a.tableView(width, max(1, height-2))
	status := ansi.Truncate(strings.ReplaceAll(a.status(), "\n", " "), width, "…")
	body := s.Muted.Render(status) + "\n" + strings.TrimRight(table, "\n")
	filter := a.filterLine(width).Offset(0, strings.Count(body, "\n")+1)
	return filter.WithText(body + "\n" + filter.Text)
}

// tableView draws the header and the visible window of rows into width x
// height cells: one header line, then height-1 rows. The window scrolls in
// both axes to keep the cursor in view; the frozen column (the first shown
// one, when Freeze is on) stays at the left.
func (a *App) tableView(width, height int) string {
	a.r.Placed(gridID)
	if width <= 0 || height <= 0 {
		return ""
	}
	blank := func(text string) string {
		lines := []string{fit(text, width)}
		for len(lines) < height {
			lines = append(lines, strings.Repeat(" ", width))
		}
		return a.r.PlacedAt(gridID, strings.Join(lines, "\n"), 0, 1)
	}
	switch {
	case a.Loading != "":
		return blank("loading " + a.Loading + "…")
	case a.LoadErr != nil:
		return blank("error: " + a.LoadErr.Error())
	case len(a.Header) == 0:
		return blank("(empty file)")
	}
	g := a.grid
	vrow, col := g.Position()
	visible := g.Visible()
	body := max(1, height-1)
	a.view.height = body
	// Vertical window, keeping the cursor row inside it.
	v := &a.view
	v.offRow = min(v.offRow, max(0, len(visible)-body))
	if vrow < v.offRow {
		v.offRow = vrow
	} else if vrow >= v.offRow+body {
		v.offRow = vrow - body + 1
	}
	// The first rows measured the columns at load; rows drawn later widen
	// them (never narrow), so a column fits what the screen has shown.
	for _, row := range visible[v.offRow:min(len(visible), v.offRow+body)] {
		for j, value := range a.Rows[row] {
			a.widen(j, value)
		}
	}
	gutter := len(strconv.Itoa(len(a.Rows))) + 1
	avail := width - gutter
	shown := a.showable()
	frozen := -1
	if a.Freeze && len(shown) > 1 {
		frozen, shown = shown[0], shown[1:]
		avail -= a.widths[frozen] + 1
	}
	v.scrolled = len(shown)
	// Horizontal window over the scrolling columns, keeping the cursor
	// column in it; a column that does not fit alone is cut.
	at := -1
	for i, j := range shown {
		if j == col {
			at = i
		}
	}
	v.offCol = min(v.offCol, max(0, len(shown)-1))
	if at >= 0 {
		if at < v.offCol {
			v.offCol = at
		}
		for v.offCol < at && a.span(shown, v.offCol, at) > avail {
			v.offCol++
		}
	}
	last := v.offCol
	for last+1 < len(shown) && a.span(shown, v.offCol, last+1) <= avail {
		last++
	}
	cols := []int{}
	if frozen >= 0 {
		cols = append(cols, frozen)
	}
	if len(shown) > 0 {
		cols = append(cols, shown[v.offCol:last+1]...)
		// A next column that does not fit whole is shown cut while there
		// is room for a few cells of it.
		if last+1 < len(shown) && avail-a.span(shown, v.offCol, last) >= 5 {
			cols = append(cols, shown[last+1])
		}
	}
	v.shown = cols
	widths := make([]int, len(cols))
	room := width - gutter
	for i, j := range cols {
		widths[i] = min(a.widths[j], max(1, room-1))
		if i == len(cols)-1 {
			// The last column takes the rest of the line instead of
			// stopping at the width cap while the line has room.
			widths[i] = max(1, room-1)
		}
		room -= widths[i] + 1
	}
	selected := map[grid.Ref]bool{}
	for _, ref := range g.Selected() {
		selected[ref] = true
	}
	focused := g.Focused()
	needle := strings.ToLower(a.Search)
	lines := make([]string, 0, height)
	var head strings.Builder
	head.WriteString(strings.Repeat(" ", gutter))
	for i, j := range cols {
		head.WriteString(" ")
		name := a.Header[j]
		if a.Sort.On && a.Sort.Col == j {
			name += map[bool]string{false: " ▲", true: " ▼"}[a.Sort.Desc]
		}
		head.WriteString(headerStyle.Render(fit(name, widths[i])))
	}
	lines = append(lines, pad(head.String(), width))
	for r := v.offRow; r < len(visible) && len(lines) < height; r++ {
		row := visible[r]
		var b strings.Builder
		b.WriteString(gutterStyle.Render(fmt.Sprintf("%*d", gutter, row+1)))
		for i, j := range cols {
			b.WriteString(" ")
			ref := grid.Ref{Row: row, Col: j}
			value := a.Rows[row][j]
			text := fit(value, widths[i])
			switch {
			case r == vrow && j == col && focused:
				text = cursorStyle.Render(text)
			case selected[ref]:
				text = selectStyle.Render(text)
			case needle != "" && strings.Contains(strings.ToLower(value), needle):
				text = matchStyle.Render(text)
			}
			b.WriteString(text)
		}
		lines = append(lines, pad(b.String(), width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	focus := 0
	if vrow >= v.offRow && 1+vrow-v.offRow < height {
		focus = 1 + vrow - v.offRow
	}
	return a.r.PlacedAt(gridID, strings.Join(lines, "\n"), focus, 1)
}

// span is the width of shown[from..to] with one space before each column.
func (a *App) span(shown []int, from, to int) int {
	total := 0
	for i := from; i <= to; i++ {
		total += a.widths[shown[i]] + 1
	}
	return total
}

// status is the line above the table: the file, the counts, the cursor's
// position and column, and whatever view state is active.
func (a *App) status() string {
	parts := []string{a.describe()}
	if a.Dirty {
		parts[0] += " [+]"
	}
	parts = append(parts, a.modeWord())
	if a.Loading != "" || len(a.Header) == 0 {
		return strings.Join(parts, " · ")
	}
	g := a.grid
	vrow, col := g.Position()
	shown := len(a.showable())
	if g.VisibleCount() > 0 {
		where := fmt.Sprintf("row %s/%s", count(vrow+1), count(g.VisibleCount()))
		if g.VisibleCount() != len(a.Rows) {
			where += fmt.Sprintf(" (of %s)", count(len(a.Rows)))
		}
		parts = append(parts, where)
	} else {
		parts = append(parts, fmt.Sprintf("no rows (of %s)", count(len(a.Rows))))
	}
	cols := fmt.Sprintf("col %d/%d %s", col+1, len(a.Header), a.columnName(col))
	if hidden := len(a.Header) - shown; hidden > 0 {
		cols += fmt.Sprintf(" (%d hidden)", hidden)
	}
	parts = append(parts, cols)
	if a.Sort.On {
		parts = append(parts, "sort "+a.columnName(a.Sort.Col)+map[bool]string{false: " ▲", true: " ▼"}[a.Sort.Desc])
	}
	if a.Filter.Query != "" {
		parts = append(parts, fmt.Sprintf("filter %s~%q", a.columnName(a.Filter.Col), a.Filter.Query))
	}
	if a.Search != "" {
		parts = append(parts, "/"+a.Search)
	}
	if a.Freeze {
		parts = append(parts, "frozen")
	}
	if marks := g.Marks(); len(marks) > 0 {
		parts = append(parts, fmt.Sprintf("%d marked", len(marks)))
	}
	return strings.Join(parts, " · ")
}

func (a *App) modeWord() string {
	if a.Edit {
		return "EDIT"
	}
	return "read-only"
}

// count prints n with thousands separators.
func count(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// fit pads or truncates a cell to width cells, replacing control characters.
func fit(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return '�'
		}
		return r
	}, s)
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

// pad cuts or pads a styled line to width cells.
func pad(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if strings.ContainsRune(line, '\x1b') {
		line += "\x1b[0m"
	}
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}
