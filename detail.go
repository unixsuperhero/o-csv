package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/unixsuperhero/ochazuke"
)

// The row detail: enter shows the cursor's row as one line per column
// (name, then the whole value, wrapped) in a pane beside the table, or
// below it on a narrow screen; enter again hides it. It is an ochazuke
// Detail (AddDetail): the framework keeps its scroll offset and J/K scroll
// it while the focus stays in the grid (the rows are the app noun's,
// guarded so they answer only while the detail overflows), and a new row
// starts it at the top. A Detail is not a focus stop, so the motions keep
// moving the cursor and the pane follows. How the two panes share the
// screen is render's (view.go), through the page's pane layout.

const detailID = "row-detail"

// registerDetail adds the detail under the sheet page. Its item is the
// cursor's row, so moving to another row resets the scroll.
func (a *App) registerDetail() error {
	d, err := a.r.AddDetail(detailID, func() string { return fmt.Sprint(a.grid.Cursor().Row) }, ochazuke.Parent(pageID))
	a.detail = d
	return err
}

// toggleDetail shows or hides the pane. Hidden, its window is emptied so
// J and K stop answering (they answer only while the detail overflows).
func (a *App) toggleDetail() {
	a.ShowDetail = !a.ShowDetail
	if !a.ShowDetail {
		a.detail.Window("", 0, 0)
	}
}

// detailView draws the pane: a title line naming the row (with the
// scroll position while it overflows), then the window of field lines.
func (a *App) detailView(width, height int) string {
	row := a.grid.Cursor().Row
	if width <= 0 || height <= 0 {
		return ""
	}
	if row < 0 || row >= len(a.Rows) {
		return pad(gutterStyle.Render("no row"), width) + strings.Repeat("\n"+strings.Repeat(" ", width), height-1)
	}
	body, marker := a.detail.Window(a.detailText(row, width), 0, height-1)
	title := fmt.Sprintf("row %s of %s", count(row+1), count(len(a.Rows)))
	if marker != "" {
		title += " · " + marker
	}
	lines := []string{pad(gutterStyle.Render(title), width)}
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, pad(line, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines[:height], "\n")
}

// detailText is row as "name  value" lines, every column (hidden ones
// included), the names in one column and each value wrapped under its own
// start; the cursor's column is bold.
func (a *App) detailText(row, width int) string {
	keyWidth := 4
	for _, name := range a.Header {
		keyWidth = max(keyWidth, min(ansi.StringWidth(name), 24))
	}
	room := max(8, width-keyWidth-2)
	indent := strings.Repeat(" ", keyWidth+2)
	col := a.grid.Cursor().Col
	var b strings.Builder
	for j, name := range a.Header {
		key := fit(name, keyWidth)
		if j == col {
			key = headerStyle.Render(key)
		}
		value := strings.ReplaceAll(a.Rows[row][j], "\r", "")
		wrapped := strings.Split(lipgloss.NewStyle().Width(room).Render(value), "\n")
		for i, line := range wrapped {
			if i == 0 {
				b.WriteString(key + "  " + strings.TrimRight(line, " "))
			} else {
				b.WriteString("\n" + indent + strings.TrimRight(line, " "))
			}
		}
		if j < len(a.Header)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
