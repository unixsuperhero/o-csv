package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/unixsuperhero/ochazuke"
	ocli "github.com/unixsuperhero/ochazuke/cli"
	"github.com/unixsuperhero/ochazuke/grid"
)

const cities = "testdata/cities.csv"

func open(t *testing.T, path string, edit bool) *App {
	t.Helper()
	a, err := New(Options{}, edit)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		if err := a.Load(path); err != nil {
			t.Fatal(err)
		}
	}
	a.m.Width, a.m.Height = 120, 24
	view(a) // the first frame measures the window (ctrl+d, ctrl+f)
	return a
}

func view(a *App) string { return ansi.Strip(a.m.View().Content) }

// press sends keys one by one, failing on the first error the model shows.
func press(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, key := range keys {
		Press(a.m, key)
		if a.m.Status != "" {
			t.Fatalf("key %q: %s", key, a.m.Status)
		}
	}
}

// pressErr sends one key and returns the error the model shows.
func pressErr(a *App, key string) string {
	Press(a.m, key)
	return a.m.Status
}

func at(t *testing.T, a *App, row, col int) {
	t.Helper()
	if got := a.grid.Cursor(); got != (grid.Ref{Row: row, Col: col}) {
		t.Fatalf("cursor = %v, want r%d c%d (focus %s)", got, row, col, a.r.Focus)
	}
}

func TestNavigation(t *testing.T) {
	a := open(t, cities, false)
	if a.r.Focus != "r0:c0" {
		t.Fatalf("start focus %s", a.r.Focus)
	}
	press(t, a, "j", "j", "l")
	at(t, a, 2, 1)
	press(t, a, "G")
	at(t, a, 9, 1)
	press(t, a, "g", "g")
	at(t, a, 0, 1)
	press(t, a, "3", "j", "$")
	at(t, a, 3, 6)
	press(t, a, "0", "ctrl+d")
	vh := a.view.height
	at(t, a, min(9, 3+vh/2), 0)
	press(t, a, "g", "g", "4", "|")
	at(t, a, 0, 3)
	press(t, a, "|")
	at(t, a, 0, 0)
	// g c asks for a column by name; a prefix finds it.
	press(t, a, "g", "c")
	Type(a.m, "popu")
	press(t, a, "enter")
	at(t, a, 0, 2)
	if v := view(a); !strings.Contains(v, "row 1/10 · col 3/7 population") {
		t.Fatalf("status line:\n%s", v)
	}
}

func TestHideFreezeAndHorizontalScroll(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "l", "-")
	if !a.Hidden[1] {
		t.Fatal("country not hidden")
	}
	at(t, a, 0, 2)
	press(t, a, "h")
	at(t, a, 0, 0)
	press(t, a, "l")
	at(t, a, 0, 2)
	if v := view(a); strings.Contains(v, "country") || !strings.Contains(v, "(1 hidden)") {
		t.Fatalf("hidden column drawn:\n%s", v)
	}
	press(t, a, "=")
	if len(a.Hidden) != 0 {
		t.Fatal("unhide kept columns hidden")
	}
	// A narrow screen scrolls the columns; F keeps the first one.
	a.m.Width = 40
	press(t, a, "$")
	v := view(a)
	if strings.Contains(v, "Tokyo") || !strings.Contains(v, "notes") {
		t.Fatalf("scrolled view still shows the first column:\n%s", v)
	}
	press(t, a, "F")
	v = view(a)
	if !strings.Contains(v, "Tokyo") || !strings.Contains(v, "notes") {
		t.Fatalf("frozen view:\n%s", v)
	}
}

func TestSearchNextPrevious(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "/")
	Type(a.m, "japan")
	press(t, a, "enter")
	at(t, a, 0, 1)
	press(t, a, "n")
	at(t, a, 1, 1)
	press(t, a, "n")
	at(t, a, 9, 1)
	press(t, a, "n") // wraps
	at(t, a, 0, 1)
	press(t, a, "N")
	at(t, a, 9, 1)
	if !strings.Contains(view(a), "/japan") {
		t.Fatal("status lacks the query")
	}
	if _, err := a.r.Execute(ochazuke.Request{Noun: "sheet", Verb: "search", Args: ochazuke.Arguments{"query": "no such"}}); err == nil {
		t.Fatal("a missing match is not an error")
	}
	press(t, a, "esc")
	if a.Search != "" {
		t.Fatal("esc kept the search")
	}
}

func TestSortCyclesAscDescFileOrder(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "l", "l", "s") // population
	if a.Rows[0][0] != "Bergen" || a.Rows[9][0] != "Lagos" || a.Dirty {
		t.Fatalf("ascending: first %s last %s dirty %v", a.Rows[0][0], a.Rows[9][0], a.Dirty)
	}
	if !strings.Contains(view(a), "population ▲") {
		t.Fatal("no sort marker")
	}
	press(t, a, "s")
	if a.Rows[0][0] != "Lagos" || !a.Sort.Desc {
		t.Fatalf("descending: first %s", a.Rows[0][0])
	}
	press(t, a, "s")
	if a.Rows[0][0] != "Tokyo" || a.Sort.On {
		t.Fatalf("file order: first %s sort %+v", a.Rows[0][0], a.Sort)
	}
}

func TestFilterField(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "l", "f")
	if a.r.Focus != filterID {
		t.Fatalf("f focus %s", a.r.Focus)
	}
	Type(a.m, "peru")
	press(t, a, "enter")
	if a.grid.VisibleCount() != 2 || a.r.Focus == filterID {
		t.Fatalf("filter: %d visible, focus %s", a.grid.VisibleCount(), a.r.Focus)
	}
	if !strings.Contains(view(a), `row 1/2 (of 10) · col 2/7 country · filter country~"peru"`) {
		t.Fatalf("status:\n%s", view(a))
	}
	press(t, a, "esc")
	if a.grid.VisibleCount() != 10 {
		t.Fatal("esc kept the filter")
	}
}

func TestRowDetail(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "l", "l", "l", "l", "l", "l", "enter")
	if a.r.TopModal() != detailID {
		t.Fatalf("enter opened %q", a.r.TopModal())
	}
	v := view(a)
	for _, want := range []string{"Row 1 of 10", "population", "13960000", "home to the Imperial"} {
		if !strings.Contains(v, want) {
			t.Fatalf("detail lacks %q:\n%s", want, v)
		}
	}
	// The cursor's column is selected; y copies its value.
	press(t, a, "k", "y")
	if y, _ := a.r.ReadYank(""); y.Text != "true" {
		t.Fatalf("detail yank %q", y.Text)
	}
	press(t, a, "esc")
	if a.r.TopModal() != "" || a.r.Focus != "r0:c6" {
		t.Fatalf("after esc: modal %q focus %s", a.r.TopModal(), a.r.Focus)
	}
	press(t, a, "enter", "q")
	if a.r.TopModal() != "" {
		t.Fatal("q left the detail open")
	}
}

func TestYankCellAndRowToRegistersAndClipboard(t *testing.T) {
	a := open(t, cities, false)
	var clip []string
	a.Clipboard = func(s string) error { clip = append(clip, s); return nil }
	press(t, a, "j", "j", "j", "j", "j")
	for _, q := range []ochazuke.Request{{Noun: "cell", Verb: "yank"}, {Noun: "row", Verb: "yank"}} {
		if _, err := a.r.Execute(q); err != nil {
			t.Fatal(err)
		}
		if err := ocli.Settle(a.r, nil); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"Zürich", `Zürich,Switzerland,421000,88,15,false,"Banking, ""chocolate"", lakes"`}
	if fmt.Sprint(clip) != fmt.Sprint(want) {
		t.Fatalf("clipboard = %q", clip)
	}
	if y, _ := a.r.ReadYank(""); y.Text != want[1] {
		t.Fatalf("register = %q", y.Text)
	}
	// The keys reach the same verbs.
	press(t, a, "k", "y")
	if y, _ := a.r.ReadYank(""); y.Text != "Oslo" {
		t.Fatalf("y = %q", y.Text)
	}
	press(t, a, "Y")
	if y, _ := a.r.ReadYank(""); !strings.HasPrefix(y.Text, "Oslo,Norway,") {
		t.Fatalf("Y = %q", y.Text)
	}
}

func copyFile(t *testing.T, from string) string {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(from))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Read-only, no key and no verb changes the sheet or writes the file.
func TestReadOnlyNeverWrites(t *testing.T) {
	path := copyFile(t, cities)
	before, _ := os.ReadFile(path)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	a := open(t, path, false)
	for _, key := range []string{"i", "x", "p", "d", "d", "o", "D", "u", "ctrl+r", "+", "ctrl+s", "v", "V", "t", "u", "s", "j", "s"} {
		Press(a.m, key)
		if a.r.TopModal() != "" {
			t.Fatalf("%s opened %s read-only", key, a.r.TopModal())
		}
	}
	for _, q := range []ochazuke.Request{
		{Noun: "sheet", Verb: "save"}, {Noun: "sheet", Verb: "saveAs", Args: ochazuke.Arguments{"path": path}},
		{Noun: "cell", Verb: "clear"}, {Noun: "row", Verb: "delete", Confirmed: true}, {Noun: "sheet", Verb: "undo"},
		{Noun: "column", Verb: "rename", Args: ochazuke.Arguments{"name": "x"}},
	} {
		if _, err := a.r.Execute(q); err == nil {
			t.Errorf("%s ran read-only", q.Key())
		}
	}
	if err := a.Save(path); err == nil {
		t.Error("Save wrote read-only")
	}
	if a.Dirty || len(a.Undo) != 0 {
		t.Fatalf("dirty %v undo %d", a.Dirty, len(a.Undo))
	}
	after, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(old) {
		t.Fatal("the file was written")
	}
	if v := view(a); strings.Contains(v, "cell.edit") || !strings.Contains(v, "read-only") {
		t.Fatalf("read-only footer offers edits:\n%s", v)
	}
}

func TestEditModeEditsUndoesAndSaves(t *testing.T) {
	path := copyFile(t, cities)
	a := open(t, path, false)
	press(t, a, "ctrl+e")
	if !a.Edit {
		t.Fatal("ctrl+e did not turn editing on")
	}
	press(t, a, "i")
	Press(a.m, "ctrl+u")
	Type(a.m, "Edo")
	press(t, a, "enter")
	if a.Rows[0][0] != "Edo" || !a.Dirty {
		t.Fatalf("edit: %q dirty %v", a.Rows[0][0], a.Dirty)
	}
	if msg := pressErr(a, "ctrl+e"); !strings.Contains(msg, "unsaved") {
		t.Fatalf("leaving edit mode dirty: %q", msg)
	}
	press(t, a, "u")
	if a.Rows[0][0] != "Tokyo" {
		t.Fatal("undo")
	}
	press(t, a, "ctrl+r", "j", "d", "d", "y") // y accepts the confirm
	if len(a.Rows) != 9 || a.Rows[1][0] != "Lagos" {
		t.Fatalf("row delete: %d rows, r1 %s", len(a.Rows), a.Rows[1][0])
	}
	press(t, a, "ctrl+s")
	if a.Dirty {
		t.Fatal("still dirty after save")
	}
	tab, err := ReadFile(path, Options{})
	if err != nil || len(tab.Rows) != 9 || tab.Rows[0][0] != "Edo" || tab.Rows[4][6] != `Banking, "chocolate", lakes` {
		t.Fatalf("saved: %v %q", err, tab.Rows)
	}
	press(t, a, "ctrl+e")
	if a.Edit {
		t.Fatal("ctrl+e did not turn editing off")
	}
}

func TestLoadRunsOffTheLoop(t *testing.T) {
	a := open(t, "", false)
	if _, err := a.r.Execute(ochazuke.Request{Noun: "sheet", Verb: "load", Target: pageID, Args: ochazuke.Arguments{"path": cities}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view(a), "loading testdata/cities.csv") || len(a.Rows) != 0 {
		t.Fatal("load ran on the loop")
	}
	if err := ocli.Settle(a.r, nil); err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 10 || a.r.Focus != "r0:c0" {
		t.Fatalf("after load: %d rows, focus %s", len(a.Rows), a.r.Focus)
	}
	if _, err := a.r.Execute(ochazuke.Request{Noun: "sheet", Verb: "load", Target: pageID, Args: ochazuke.Arguments{"path": "testdata/missing.csv"}}); err != nil {
		t.Fatal(err)
	}
	_ = ocli.Settle(a.r, nil)
	if !strings.Contains(view(a), "error: open testdata/missing.csv") {
		t.Fatalf("load error not shown:\n%s", view(a))
	}
}

// A 100k-row file draws one screen: a frame and a motion stay fast.
func TestBigFileDrawsOnlyTheWindow(t *testing.T) {
	var b strings.Builder
	b.WriteString("id,name,value\n")
	for i := range 100000 {
		fmt.Fprintf(&b, "%d,name %d,%d\n", i, i, i*7%1000)
	}
	path := filepath.Join(t.TempDir(), "big.csv")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a := open(t, path, false)
	start := time.Now()
	press(t, a, "G")
	v := view(a)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("G and a frame took %s", elapsed)
	}
	if !strings.Contains(v, "row 100,000/100,000") || !strings.Contains(v, "name 99999") || strings.Contains(v, "name 5 ") {
		t.Fatalf("big view:\n%s", v)
	}
}

func TestSmokeFrame(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := Main([]string{"--smoke", "--width", "100", cities}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	for _, want := range []string{"testdata/cities.csv · read-only · row 1/10 · col 1/7 city", "Tokyo", "population", "enter row.detail"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("smoke lacks %q:\n%s", want, out.String())
		}
	}
}

// Every row o-csv and the grid install is a Request to a registered
// operation or an unbind (ochazuke's TestBindingRowsAreAtomic), and none
// binds a function key.
func TestBindingRowsAreAtomic(t *testing.T) {
	for _, edit := range []bool{false, true} {
		a := open(t, cities, edit)
		if _, err := a.r.ValidateBindings(); err != nil {
			t.Fatal(err)
		}
		var rows []ochazuke.Binding
		for _, noun := range a.r.Nouns {
			for _, table := range noun.Bindings {
				rows = append(rows, table...)
			}
		}
		for _, inst := range a.r.Instances {
			rows = append(rows, inst.Bindings...)
		}
		for _, b := range rows {
			for _, key := range strings.Fields(b.Keys) {
				if len(key) >= 2 && key[0] == 'f' && key[1] >= '1' && key[1] <= '9' {
					t.Errorf("function key bound: %s", b.Keys)
				}
			}
		}
	}
}

// The TUI schedules sheet.load before the program starts; the model's first
// update hands the job to Bubble Tea and its completion fills the sheet.
func TestStartupLoadThroughTheModel(t *testing.T) {
	a, err := New(Options{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.r.Execute(ochazuke.Request{Noun: "sheet", Verb: "load", Target: pageID, Args: ochazuke.Arguments{"path": cities}}); err != nil {
		t.Fatal(err)
	}
	_, cmd := a.m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		// A timer (the jobs ticker's linger) never answers in time; skip it.
		got := make(chan tea.Msg, 1)
		go func() { got <- cmd() }()
		var m tea.Msg
		select {
		case m = <-got:
		case <-time.After(200 * time.Millisecond):
			return
		}
		switch msg := m.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case ochazuke.Completion:
			_, next := a.m.Update(msg)
			run(next)
		}
	}
	run(cmd)
	if len(a.Rows) != 10 || a.Loading != "" || !strings.Contains(view(a), "Tokyo") {
		t.Fatalf("startup load: %d rows, loading %q", len(a.Rows), a.Loading)
	}
}

func TestNamedRegisterYank(t *testing.T) {
	a := open(t, cities, false)
	press(t, a, "\"", "a", "y")
	if y, _ := a.r.ReadYank("a"); y.Text != "Tokyo" {
		t.Fatalf(`"a y stored %q`, y.Text)
	}
}

// A save that fails partway leaves the original bytes and no stray
// temporary file; a save that succeeds keeps the file's mode.
func TestSaveIsAtomic(t *testing.T) {
	path := copyFile(t, cities)
	before, _ := os.ReadFile(path)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeAtomic(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "city,coun"); err != nil {
			return err
		}
		return errors.New("disk full")
	})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("failed write returned %v", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatalf("the original was changed:\n%s", after)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
	a := open(t, path, true)
	press(t, a, "i")
	Press(a.m, "ctrl+u")
	Type(a.m, "Edo")
	press(t, a, "enter", "ctrl+s")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("saved mode %v, %v", info.Mode(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
	if tab, err := ReadFile(path, Options{}); err != nil || tab.Rows[0][0] != "Edo" || len(tab.Rows) != 10 {
		t.Fatalf("saved: %v", err)
	}
	// saveAs into a directory that cannot be written fails and writes nothing.
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(filepath.Join(dir, "out.csv")); err == nil {
		t.Fatal("save into an unwritable directory succeeded")
	}
	if a.Path != path {
		t.Fatalf("a failed saveAs moved the sheet to %s", a.Path)
	}
}
