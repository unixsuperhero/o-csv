package main

import (
	"github.com/unixsuperhero/ochazuke"
	"github.com/unixsuperhero/ochazuke/grid"
)

// bind installs the key rows. With a cell focused the stack is [cell, row,
// column, grid, sheet, app]: the cell answers the motions, search, copy and
// detail; the column sorts, hides and filters; the sheet owns the view
// toggles, editing and saving. Edit rows are guarded (verbs.go): read-only
// they do not answer, so their keys are free and the footer leaves them
// out. Counts reach the motions through count_pending, which is exclusive,
// so the cell's count_pending extends its normal table.
func (a *App) bind() {
	r := a.r
	move := func(motion string) ochazuke.Binding {
		return ochazuke.Do("sheet.move", ochazuke.Arguments{"motion": motion}).FromPayload("steps", ochazuke.SourceCount)
	}
	line := func(motion string) ochazuke.Binding {
		return ochazuke.Do("sheet.move", ochazuke.Arguments{"motion": motion}).FromPayload("line", ochazuke.SourceLine)
	}
	motions := func(m *ochazuke.ModeBuilder) {
		m.Bind("h", move("left")).Bind("left", move("left")).
			Bind("l", move("right")).Bind("right", move("right")).
			Bind("j", move("down")).Bind("down", move("down")).
			Bind("k", move("up")).Bind("up", move("up")).
			Bind("0", move("row-start")).Bind("home", move("row-start")).
			Bind("$", move("row-end")).Bind("end", move("row-end")).
			Bind("ctrl+d", move("half-down")).Bind("ctrl+u", move("half-up")).
			Bind("ctrl+f", move("page-down")).Bind("ctrl+b", move("page-up")).
			Bind("g g", line("top")).Bind("G", line("bottom"))
	}
	shape := func(s string) ochazuke.Binding {
		return ochazuke.Do("mode.set", ochazuke.Arguments{"mode": s}).Guard(guardEditGrid)
	}
	transforms := func(m *ochazuke.ModeBuilder, noun string) {
		for _, kn := range [][2]string{{"t u", "upper"}, {"t l", "lower"}, {"t t", "trim"}, {"t i", "title"}, {"t s", "slug"}} {
			m.Bind(kn[0], ochazuke.Do(noun+".transform", ochazuke.Arguments{"name": kn[1]}))
		}
	}

	cell := r.Define("cell").Mode("normal").
		Bind("|", ochazuke.Do("sheet.column").FromPayload("index", ochazuke.SourceCount)).
		Bind("g c", ochazuke.Do("sheet.column")).
		Bind("/", ochazuke.Do("sheet.search")).
		Bind("n", ochazuke.Do("sheet.next")).
		Bind("N", ochazuke.Do("sheet.previous")).
		Bind("enter", ochazuke.Do("row.detail")).
		Bind("y", ochazuke.Do("cell.yank")).
		Bind("Y", ochazuke.Do("row.yank")).
		// Edit mode.
		Bind("i", ochazuke.Do("cell.edit")).
		Bind("x", ochazuke.Do("cell.clear")).
		Bind("p", ochazuke.Do("cell.paste")).
		Bind("v", shape(grid.ShapeBlock)).Bind("V", shape(grid.ShapeRow)).Bind("ctrl+v", shape(grid.ShapeColumn))
	motions(cell)
	transforms(cell, "cell")
	r.Define("cell").Mode("count_pending").Extends("normal")

	r.Define("row").Mode("normal").
		Bind("d d", ochazuke.Do("row.delete")).
		Bind("o", ochazuke.Do("row.insertBelow"))

	r.Define("column").Mode("normal").
		Bind("s", ochazuke.Do("column.sort")).
		Bind("-", ochazuke.Do("column.hide")).
		Bind("f", ochazuke.Do("column.filter")).
		Bind("r", ochazuke.Do("column.rename"))

	visual := r.Define("grid")
	visual.Mode(grid.ShapeRow).Extends(grid.ShapeBlock)
	visual.Mode(grid.ShapeColumn).Extends(grid.ShapeBlock)
	m := visual.Mode(grid.ShapeBlock)
	motions(m)
	transforms(m, "selection")
	m.Bind("o", ochazuke.Do("selection.swap")).
		Bind("v", shape(grid.ShapeBlock)).Bind("V", shape(grid.ShapeRow)).Bind("ctrl+v", shape(grid.ShapeColumn)).
		Bind("esc", ochazuke.Do("mode.set", ochazuke.Arguments{"mode": "normal"})).
		Bind("y", ochazuke.Do("selection.yank")).
		Bind("d", ochazuke.Do("selection.delete")).
		Bind("p", ochazuke.Do("selection.paste")).
		Bind("c", ochazuke.Do("selection.clear")).
		Bind("f", ochazuke.Do("selection.transform", ochazuke.Arguments{"name": "fillDown"})).
		Bind("m", ochazuke.Do("selection.mark")).
		Bind("M", ochazuke.Do("selection.unmark")).
		Bind("ctrl+a", ochazuke.Do("selection.all")).
		Bind(":", ochazuke.Do("palette.open")).
		Pattern("digit-0", ochazuke.Enter("count_pending").FromPayload("count", ochazuke.SourceDigit))

	r.Define("sheet").Mode("normal").
		Bind("=", ochazuke.Do("sheet.unhide")).
		Bind("F", ochazuke.Do("sheet.freeze")).
		Bind("esc", ochazuke.Do("sheet.clear")).
		Bind("ctrl+e", ochazuke.Do("sheet.edit")).
		Bind("ctrl+o", ochazuke.Do("sheet.open")).
		Bind("ctrl+s", ochazuke.Do("sheet.save")).
		Bind("+", ochazuke.Do("sheet.addColumn")).
		Bind("u", ochazuke.Do("sheet.undo")).
		Bind("ctrl+r", ochazuke.Do("sheet.redo"))
}

// instanceRows are the rows the grid puts on its focus instances: D on the
// column instance deletes that column with its header as the argument the
// confirm prompt names. A noun table cannot carry the header; the instance
// row is rebuilt on every sync.
func (a *App) instanceRows(kind string, at grid.Ref) []ochazuke.Binding {
	if kind != "column" || at.Col < 0 || at.Col >= len(a.Header) {
		return nil
	}
	row := ochazuke.Do("column.delete", ochazuke.Arguments{"name": a.Header[at.Col]})
	row.Mode, row.Keys = "normal", "D"
	return []ochazuke.Binding{row}
}
