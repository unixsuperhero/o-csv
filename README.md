# o-csv

A CSV viewer for the o family, built on [ochazuke](../ochazuke) (Bubble Tea v2) with an [oden](../oden) command line. It started as a copy of ochazuke's `examples/csv` editor. Viewing comes first. Editing is still there, but it stays off until you ask for it.

```sh
o-csv data.csv                  # open a file
some-command | o-csv            # or read stdin (also: o-csv -)
o-csv --delim tab --no-header x.txt
o-csv --edit data.csv           # start with editing on
o-csv --smoke data.csv          # render one 80x24 frame to stdout and exit
```

Build: `go build -o ~/bin/o-csv .`

## The viewer

The table fills the screen: a header row, then only the rows that fit on screen. That means a 100k-row file costs one screen per frame, and it loads off the event loop (`sheet.load` is pending work, and the screen says `loading …` while it runs). Each column is as wide as the widest of its first 2,000 values and of the rows drawn so far, capped at 32 cells (the `column-width` setting). Longer cells are cut with `…`. On a narrow screen the columns scroll with the cursor, and the next column shows partly while there is room for it. The status line above the table shows the file, `read-only` or `EDIT`, `row N/M` (with `(of TOTAL)` while a filter is on), `col N/M NAME`, and the active sort, filter, search, frozen column and hidden-column count. The delimiter is guessed from the first 64 KiB, trying comma, tab, semicolon and pipe. `--delim` (or the `delimiter` setting) overrides it. With `--no-header`, the columns are named A, B, C, …

### Keys

`?` lists every key the focus answers to, generated from the binding rows. The footer shows the keys that are not motions.

| Key | Does |
|---|---|
| `h j k l`, arrows | move (counts work: `5j`) |
| `0` `$` / `home` `end` | first / last column |
| `g g` / `G` | first / last row (`12G` goes to row 12) |
| `ctrl+d` `ctrl+u` | half a screen down / up |
| `ctrl+f` `ctrl+b` | a screen down / up |
| `\|` | column by number (`4\|`; plain `\|` is the first column) |
| `g c` | column by name: you type it, and an exact match wins, then a prefix, then a substring |
| `/` | search every shown cell (case-insensitive); matches are underlined |
| `n` / `N` | next / previous match, wrapping |
| `f` | filter rows by the cursor's column in the field below the table: `enter` applies it, `esc` or `tab` goes back |
| `esc` | clear the search and the filter |
| `s` | sort by the cursor's column: ascending, then descending, then back to file order. Numbers sort numerically and empty cells go last |
| `-` / `g =` | hide the cursor's column / show every column again |
| `F` | freeze the first shown column at the left (toggle) |
| `enter` | show or hide the row detail: every column of the cursor's row with its whole value, wrapped, beside the table (below it under 100 columns). It follows the cursor; the focus stays in the table |
| `J` / `K` | scroll the row detail while it is longer than its pane (counts work: `5J`) |
| `>` / `<` / `=` | while the detail shows: grow the table, shrink it, put the divider back (counts work: `3>`). The divider is remembered in `~/.local/state/o-csv/panes.json` |
| `y` / `Y` | copy the cell / copy the row as one CSV line. The copy goes to the ochazuke registers (`"a y` names one) and to the system clipboard (pbcopy, wl-copy, xclip or xsel, whichever is on PATH). `.` yanks again at the cursor (`y j .` copies the next cell) |
| `ctrl+o` | open another file |
| `:` | the command palette (every verb, including those without a key) |
| `ctrl+c` | quit |

### Editing

Editing is off by default. Start with `--edit` or press `ctrl+e` to toggle it. Leaving edit mode with unsaved edits is refused. While editing is off, every edit verb is guarded: its key does nothing, the palette does not offer it, and the sheet refuses it underneath. Nothing in a read-only session writes a file (`TestReadOnlyNeverWrites`).

While editing is on, ochazuke's CSV example keys apply: `i` edit a cell, `x` clear, `p` paste, `d d` delete a row (it asks first), `o` insert a row, `r` rename a column, `D` delete a column, `+` add a column, `t u`/`t l`/`t t`/`t i`/`t s` transform, `v`/`V`/`ctrl+v` visual block, row and column selections, `u` / `ctrl+r` undo and redo, `ctrl+s` save. A save keeps the delimiter the file was read with. Every edit is one patch, and its inverse is the undo record. Sorting while editing is an undoable edit. Read-only, a sort only reorders the view.

## Settings

Two knobs live in `~/.config/o-csv/settings.json` (`$XDG_CONFIG_HOME/o-csv`), declared as ochazuke settings, so the viewer and the command line read the same file:

| Setting | Default | Does |
|---|---|---|
| `delimiter` | `auto` | the field delimiter when `--delim` is not given: `auto` (detect), `comma`, `tab`, `semicolon`, `pipe`, or one character |
| `column-width` | `32` | the widest a column grows in the viewer, in cells (3 to 200) |

`o-csv settings list`, `o-csv settings set column-width 24` and `o-csv settings reset delimiter` change them; in the viewer `: settings edit` opens the settings editor and a change applies at once. `o-csv config export` writes a reference of every setting and key row into `~/.config/o-csv/reference/`.

## Command line

`o-csv table VERB FILE [--ARG VALUE]` reads the file in the process and prints JSON. `-` reads stdin. Every verb takes `--delim` and `--no_header`. `--format table|markdown|toon` prints the rows of `table show` (and `table columns`) as a table instead: `table show` draws the CSV's own columns, numbers right-aligned. `--json '{...}'` passes the arguments as JSON instead, and a bare `--json` is accepted and ignored.

| Command | Prints |
|---|---|
| `o-csv table show FILE` | the matching rows, each `{noun: "row", id: "row/N", data, cells}`: `N` is its row in the file (whatever the sort), `data` the row as an object keyed by column in header order, `cells` its values. When `--offset`/`--limit` leave rows out, stderr says `status: rows 11–20 of 250`. Options: `--columns a,b`, `--search TEXT`, `--where COL~TEXT`, `--sort COL [--desc]`, `--offset N`, `--limit N` |
| `o-csv table columns FILE` | `[{index, name, type}]` |
| `o-csv table stats FILE` | the row and column counts, the delimiter, and per column: the guessed type (empty, bool, int, float, date, string), empty and distinct counts, min, max, widest value |
| `o-csv catalog` | the runtime catalog for a host that mounts the bin |

`o-csv help` lists the commands, and `o-csv table --help` lists the verbs and their parameters.

## How o-csv is built (a reference app)

o-csv is the family's reference for building an application on the current ochazuke (v0.9.0): a separate module that imports `github.com/unixsuperhero/ochazuke`, owns its state, and gets keys, help, the palette, the jobs ticker, settings and a CLI from the framework. Read the files in this order; each one shows one concept.

| Concept | Where | What to look at |
|---|---|---|
| The app owns its state | `app.go` | `App` holds the file, the view state and the undo stack; the runtime holds only interaction state, and instances carry ids, never data. `Patch` is the one edit shape, and its inverse is the undo record. |
| Domain code without the framework | `sheet.go` | Reading, writing, delimiter detection, type guesses and stats are plain Go, tested on their own. |
| Building the tree | `main.go` (`New`) | `NewRuntime`, an instance for the page, a component under it (`grid.New`), `AddDetail`, an input (`filter.go`), `AddPage`, `OpenPage`, `NewModel`. |
| Nouns, verbs, guards, confirms | `verbs.go` | `Define(noun).Verb(verb, handler, description)`; `.When(guard)` hides and skips a verb (`DefineGuard("edit", …)`), `.Confirm("Delete row {{id}}?")` asks first, `.Motion()` keeps a verb off the footer. Every verb declares its history kind: `.Fetch()` on a read (`sheet.state`, the `table` verbs), `setHistory(r, ochazuke.HistoryChange|HistoryAction, keys...)` for edits and view moves, so none falls to the default (`history_test.go`). `setRepeatable` marks the yanks `Operation.Repeatable`, so `.` repeats them. Handlers are `func(Ctx) Result` (`Done`, `Failed`, `Pending`). |
| Binding rows as data | `bindings.go`, `filter.go` | Every key is a row: `Do("noun.verb", args)`, arguments from the key's payload (`FromPayload("steps", SourceCount)`), a pattern row for digits, mode tables that extend others, instance rows (`grid.WithInstanceRows`, `r.Bind`). No key-name switch anywhere; `TestBindingRowsAreAtomic` runs `ValidateBindings`. |
| Work off the loop | `verbs.go` (`sheet.load`, `copyText`), `shell.go` | `Pending(work)` runs I/O as a job; `.Replacing("load")` lets a newer load cancel an older one; `.Then` runs on the loop when it lands. Reads are `.Quiet()` so they stay off the ticker. |
| Telling the user | `verbs.go` (`copyText`), `app.go` (`saveTo`) | `ctx.Status("copied …")`, `ctx.Notify(…)` / `ctx.ClearNotice(id)` for a lasting problem, `r.UI().Status` where there is no `Ctx`. The TUI shows them in the footer; the CLI prints `status:` and `notice:` on stderr. |
| A page detail | `detail.go` | `AddDetail(id, item)` and `Detail.Window`: the framework keeps the scroll offset, `J`/`K` scroll it while the focus stays in the grid, and a new item starts at the top. |
| Resizable panes | `view.go` (`sheetView`) | `r.PaneLayout(page, layout)` and `Layout.Divide` make `>` `<` `=` move the divider, remembered per page; the sheet keeps its own rows off those keys. |
| Settings | `settings.go` | `DeclareSetting` with a `Validate`, read with `Settings().String/Int`, followed with `Observe`. `settings list|set`, the settings editor and `config export` cover them. |
| Modals | `prompt.go` | `OpenModal(kind, Payload, WithInput(in, Kind("field")))` for a one-line question; the noun pins insert mode, and `enter`/`esc` are rows on it. |
| Drawing a component's view | `view.go` (`tableView`) | The grid owns the cursor and the visible rows; the app draws only the window and calls `Placed`/`PlacedAt`, so motions and scrolling follow the screen. |
| The CLI and the catalog | `shell.go`, `main.go` (`Main`) | A second runtime with the `table` noun behind `ocli.New`, mounted on oden with `ocli.Mount`: parameters, `.Positional("path")`, `.Rows("row")`, rows in the `{noun, id, data, cells}` shape and the noun's columns, so `--format table|markdown|toon` work. `o-csv catalog` publishes it. |
| Tests | `app_test.go`, `shell_test.go`, `keys_test.go` | Drive the `Model` with key names (`Press`, `runCmds` for jobs), run the CLI through `Main`, check the frame with `--smoke`. |
