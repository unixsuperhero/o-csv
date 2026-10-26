package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/unixsuperhero/ochazuke"
)

// The row detail: enter on a cell opens the row as a modal table of column
// and value, one row per column of the file (hidden columns included). The
// rows are a list of the value noun, so j/k move and the one highlighted
// row is the list's (read-only: it takes no marks); y copies the value; esc (the modal noun), q or enter
// close it. Below the table the selected value is printed in full, wrapped,
// because the table cuts long cells.

const (
	detailID   = "row-detail"
	detailList = "row-detail/fields"
)

func (a *App) registerDetail() {
	r := a.r
	r.Define("value").
		Verb("yank", func(c ochazuke.Ctx) ochazuke.Result {
			v, _ := c.Instance.Data.(string)
			return a.copyText(v, ochazuke.StringArg(c.Args, "register", ""))
		}, "Copy the value into the registers and the system clipboard").
		Mode("normal").Bind("y", ochazuke.Do("value.yank"))
	r.Define(detailID).Mode("normal").
		Bind("q", ochazuke.Do("modal.pop")).
		Bind("enter", ochazuke.Do("modal.pop"))
	a.m.ModalViews[detailID] = ochazuke.ModalView{Title: "Row", Render: a.detailView}
}

// openDetail opens the modal for row i.
func (a *App) openDetail(i int) error {
	r := a.r
	if _, err := r.OpenModal(detailID, ochazuke.Payload{Prompt: fmt.Sprintf("Row %s of %s", count(i+1), count(len(a.Rows)))}); err != nil {
		return err
	}
	keyWidth := 6
	items := make([]ochazuke.ListItem, len(a.Header))
	for j, name := range a.Header {
		keyWidth = max(keyWidth, ansi.StringWidth(name))
		value := a.Rows[i][j]
		items[j] = ochazuke.ListItem{ID: fmt.Sprintf("%s/%d", detailID, j), Label: name, Noun: "value", Data: value, Cells: []string{name, value}}
	}
	cols := ochazuke.Columns(ochazuke.Column{Title: "column", Width: min(keyWidth, 28)}, ochazuke.Column{Title: "value"})
	list, err := r.AddList(detailList, items, ochazuke.Parent(detailID), cols, ochazuke.NoSpill(), ochazuke.NoMarks())
	if err != nil {
		_ = r.Pop()
		return err
	}
	view := a.m.ModalViews[detailID]
	view.Title = fmt.Sprintf("Row %s of %s", count(i+1), count(len(a.Rows)))
	a.m.ModalViews[detailID] = view
	if j := a.grid.Cursor().Col; j >= 0 && j < len(items) {
		_ = list.Select(items[j].ID)
	}
	return list.Focus()
}

// detailView draws the field table with the selected value wrapped below.
func (a *App) detailView(r *ochazuke.Runtime, _ *ochazuke.Instance, w, h int) ochazuke.Block {
	list := r.List(detailList)
	if list == nil {
		return ochazuke.Text("")
	}
	item, _ := list.Selected()
	value, _ := item.Data.(string)
	wrapped := ""
	room := min(6, max(0, h/3))
	if room > 0 && ansi.StringWidth(value) > max(1, w-30) {
		lines := strings.Split(lipgloss.NewStyle().Width(max(1, w)).Render(value), "\n")
		if len(lines) > room-1 {
			lines = append(lines[:room-2], "…")
		}
		wrapped = "\n" + strings.Repeat("─", max(1, w)) + "\n" + strings.Join(lines, "\n")
	}
	table := list.View(w, max(1, h-strings.Count(wrapped, "\n")))
	return ochazuke.Text(strings.TrimRight(table, "\n") + wrapped)
}
